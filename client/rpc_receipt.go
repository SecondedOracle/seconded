package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	if !robinhoodNetwork(normalizedNetwork(e.Network)) {
		debugRecovery("locator_network_ineligible", nil)
		return "", nil
	}
	if e.Receipt == nil {
		debugRecovery("locator_receipt_missing", nil)
		return "", nil
	}
	if e.Receipt != nil && e.Receipt.Envelope.State != "final" {
		debugRecovery("locator_receipt_not_final", nil)
		return "", nil
	}
	if err := (ReceiptVerifier{labels: releaseLabels}).VerifyStored(*e.Receipt, e, time.Now()); err != nil {
		debugRecovery("locator_verification_failed", err)
		return "", err
	}
	b := e.Receipt.Envelope.Billing
	if b.Tx == nil || !hexWord.MatchString(*b.Tx) || b.Charged != "yes" || b.Settlement != "final" {
		debugRecovery("locator_billing_invalid", nil)
		return "", ErrInvalid
	}
	debugRecovery("locator_verified", nil)
	return *b.Tx, nil
}

type receiptCertificate struct {
	Evidence                      ChainEvidence
	BlockNumber, TxIndex          uint64
	AuthorizationLog, TransferLog uint64
	BlockHash                     string
}

// Each independent reader must prove the exact token transfer and nonce use
// inside the same successful, canonical transaction covered by finality.
// No server statement or unavailable archive query supplies chain evidence.
func (q *Quorum) receiptEvidence(ctx context.Context, e Entry, s ChainSnapshot, tx string) (ChainEvidence, error) {
	if !q.publicPair() {
		debugRecovery("receipt_pair_unconfigured", nil)
		return ChainEvidence{}, errEvidenceNotConfigured
	}
	certificates := make([]receiptCertificate, len(q.rpcs))
	if err := q.readRecentReaders(func(i int, r *RPC) (err error) {
		certificates[i], err = r.receiptCertificate(ctx, e, s, tx)
		return
	}); err != nil {
		debugRecovery("receipt_reader_failed", err)
		return ChainEvidence{}, err
	}
	first := certificates[0]
	for _, certificate := range certificates[1:] {
		if certificate != first {
			debugRecovery("receipt_pair_disagreement", nil)
			return ChainEvidence{}, errEvidenceDisagreement
		}
	}
	if err := q.recheckRecentSnapshot(ctx, s); err != nil {
		debugRecovery("receipt_snapshot_changed", err)
		return ChainEvidence{}, err
	}
	// Recheck the payment block after both proofs, closing a reorg during reads.
	if err := q.readRecentReaders(func(_ int, r *RPC) error {
		block, err := r.block(ctx, fmt.Sprintf("0x%x", first.BlockNumber))
		if err != nil {
			return err
		}
		if block.Number != first.BlockNumber || block.Hash != first.BlockHash || block.Timestamp != first.Evidence.SpentAt {
			return errEvidenceDisagreement
		}
		return nil
	}); err != nil {
		debugRecovery("receipt_block_recheck_failed", err)
		return ChainEvidence{}, err
	}
	debugRecovery("receipt_pair_verified", nil)
	return first.Evidence, nil
}

func (r *RPC) receiptCertificate(ctx context.Context, e Entry, s ChainSnapshot, tx string) (out receiptCertificate, err error) {
	defer func() { debugSettlement(r, "eth_getTransactionReceipt", err, "certificate") }()
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
	var block, finalized ChainSnapshot
	if err := parallelReads(
		func() (err error) { block, err = r.block(ctx, number); return },
		func() (err error) { finalized, err = r.block(ctx, "finalized"); return },
	); err != nil {
		return out, err
	}
	if block.Number != blockNumber || block.Hash != blockHash || block.Timestamp <= 0 || block.Timestamp > s.Timestamp {
		return out, ErrInvalid
	}
	if finalized.Number < s.Number || finalized.Timestamp < s.Timestamp ||
		(finalized.Number == s.Number && (finalized.Hash != s.Hash || finalized.Timestamp != s.Timestamp)) {
		return out, errEvidenceDisagreement
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
