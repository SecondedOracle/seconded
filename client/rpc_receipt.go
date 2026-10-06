package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"
)

func robinhoodNetwork(network string) bool {
	return network == "eip155:4663" || network == "eip155:46630"
}

// A signed FINAL receipt supplies only a transaction locator. It never supplies
// the independent chain certificate that changes a reservation to SPENT.
func receiptTransactionLocator(e Entry) (string, error) {
	if !robinhoodNetwork(normalizedNetwork(e.Network)) || e.Receipt == nil || e.Receipt.Envelope.State != "final" {
		return "", nil
	}
	if err := (ReceiptVerifier{labels: releaseLabels}).VerifyStored(*e.Receipt, e, time.Now()); err != nil {
		return "", err
	}
	b := e.Receipt.Envelope.Billing
	if b.Tx == nil || !hexWord.MatchString(*b.Tx) || b.Charged != "yes" || b.Settlement != "final" {
		return "", ErrInvalid
	}
	return *b.Tx, nil
}

type receiptCertificate struct {
	Evidence                      ChainEvidence
	BlockNumber, TxIndex          uint64
	AuthorizationLog, TransferLog uint64
	BlockHash                     string
}

// Only an explicit archive-policy rejection permits this alternative proof.
// Every other history error, and any disagreement with a successful history
// certificate, remains a failure. The original history path is unchanged.
func (q *Quorum) receiptFallback(ctx context.Context, e Entry, s ChainSnapshot, prior []ChainEvidence, failures []error) (ChainEvidence, error) {
	for i, err := range failures {
		if err == nil {
			continue
		}
		var read *rpcReadError
		if !errors.Is(err, ErrReaderHistoryUnavailable) || !errors.As(err, &read) || read.method != "eth_getLogs" || !prior[i].authorizationUsed {
			return ChainEvidence{}, err
		}
	}
	tx, err := receiptTransactionLocator(e)
	if err != nil {
		return ChainEvidence{}, err
	}
	if tx == "" {
		return ChainEvidence{}, ErrReaderHistoryUnavailable
	}
	certificates := make([]receiptCertificate, len(q.rpcs))
	if err := q.readRecentReaders(func(i int, r *RPC) (err error) {
		certificates[i], err = r.receiptCertificate(ctx, e, s, tx)
		return
	}); err != nil {
		return ChainEvidence{}, err
	}
	first := certificates[0]
	for i, certificate := range certificates {
		if certificate != first || (failures[i] == nil && prior[i] != certificate.Evidence) {
			return ChainEvidence{}, errEvidenceDisagreement
		}
	}
	if err := q.recheckRecentSnapshot(ctx, s); err != nil {
		return ChainEvidence{}, err
	}
	return first.Evidence, nil
}

func (r *RPC) receiptCertificate(ctx context.Context, e Entry, s ChainSnapshot, tx string) (receiptCertificate, error) {
	var out receiptCertificate
	if !robinhoodNetwork(r.network) || normalizedNetwork(e.Network) != r.network || normalizedNetwork(s.Network) != r.network {
		return out, ErrInvalid
	}
	var raw map[string]json.RawMessage
	if err := r.call(ctx, "eth_getTransactionReceipt", []any{tx}, &raw); err != nil {
		return out, err
	}
	// These are the Ethereum receipt fields plus Robinhood's Arbitrum fields.
	allowed := strings.Fields("transactionHash transactionIndex blockHash blockNumber from to cumulativeGasUsed gasUsed contractAddress logs logsBloom status effectiveGasPrice type gasUsedForL1 l1BlockNumber")
	for name := range raw {
		if !oneOf(name, allowed...) {
			return out, ErrInvalid
		}
	}
	var actual, blockHash, number, status, transactionIndex string
	for field, value := range map[string]*string{"transactionHash": &actual, "blockHash": &blockHash, "blockNumber": &number, "status": &status, "transactionIndex": &transactionIndex} {
		if json.Unmarshal(raw[field], value) != nil {
			return out, ErrInvalid
		}
	}
	blockNumber, err := quantity(number)
	if err != nil || actual != tx || !hexWord.MatchString(blockHash) || status != "0x1" || blockNumber < e.StartBlock || blockNumber > s.Number {
		return out, ErrInvalid
	}
	txIndex, err := quantity(transactionIndex)
	if err != nil {
		return out, ErrInvalid
	}
	block, err := r.block(ctx, number)
	if err != nil {
		return out, err
	}
	if block.Number != blockNumber || block.Hash != blockHash || block.Timestamp <= 0 || block.Timestamp > s.Timestamp {
		return out, ErrInvalid
	}
	var logs []chainLog
	if DecodeStrict(raw["logs"], &logs, ResponseLimit) != nil || len(logs) == 0 {
		return out, ErrInvalid
	}
	usedTopic := "0x" + hex.EncodeToString(keccak([]byte("AuthorizationUsed(address,bytes32)")))
	cancelTopic := "0x" + hex.EncodeToString(keccak([]byte("AuthorizationCanceled(address,bytes32)")))
	transferTopic := "0x" + hex.EncodeToString(keccak([]byte("Transfer(address,address,uint256)")))
	payerTopic := "0x" + hex.EncodeToString(addressWord(e.Payer))
	payeeTopic := "0x" + hex.EncodeToString(addressWord(PayTo))
	seen := map[uint64]bool{}
	used, transfers := 0, 0
	for _, l := range logs {
		n, nerr := quantity(l.BlockNumber)
		i, ierr := quantity(l.Index)
		ti, terr := quantity(l.TxIndex)
		if nerr != nil || ierr != nil || terr != nil || n != blockNumber || ti != txIndex || l.BlockHash != blockHash || l.Tx != tx || l.Removed || seen[i] || !addressPattern.MatchString(strings.ToLower(l.Address)) || len(l.Topics) > 4 {
			return out, ErrInvalid
		}
		seen[i] = true
		for _, topic := range l.Topics {
			if !hexWord.MatchString(topic) {
				return out, ErrInvalid
			}
		}
		if strings.ToLower(l.Address) != pins(r.network).Asset || len(l.Topics) == 0 {
			continue
		}
		if l.Topics[0] == usedTopic || l.Topics[0] == cancelTopic {
			if len(l.Topics) != 3 || l.Data != "0x" {
				return out, ErrInvalid
			}
			if l.Topics[1] == payerTopic && l.Topics[2] == e.Nonce {
				if l.Topics[0] == cancelTopic {
					return out, ErrInvalid
				}
				used++
				out.AuthorizationLog = i
			}
		}
		if l.Topics[0] == transferTopic {
			if len(l.Topics) != 3 || !hexWord.MatchString(l.Data) {
				return out, ErrInvalid
			}
			if l.Topics[1] == payerTopic && l.Topics[2] == payeeTopic {
				amount, ok := new(big.Int).SetString(l.Data[2:], 16)
				if !ok || !amount.IsInt64() || amount.Int64() != e.Amount {
					return out, ErrInvalid
				}
				transfers++
				out.TransferLog = i
			}
		}
	}
	if used != 1 || transfers != 1 {
		return out, ErrInvalid
	}
	out.BlockNumber, out.BlockHash, out.TxIndex = blockNumber, blockHash, txIndex
	out.Evidence = ChainEvidence{State: "SPENT", SpentAt: block.Timestamp, Tx: tx, authorizationUsed: true}
	return out, nil
}
