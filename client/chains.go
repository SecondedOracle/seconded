package client

import (
	"context"
	"fmt"
	"os"
)

// This table is checked against server/payments/networks.py by the parity test.
// RPC URLs and independent operators remain release pins; server responses
// cannot replace them. Web PKI authenticates each reader across key renewals.
type ChainPins struct {
	Network         string
	ChainID         int64
	Asset           string
	Name            string
	Version         string
	FinalizedMaxAge int64
	HistoryBlocks   uint64
	BlockFields     []string
	Endpoints       []EvidenceEndpoint
	Testnet         bool
}

var chainPins = map[string]ChainPins{
	"eip155:8453":    {"eip155:8453", 8453, "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", "USD Coin", "2", 1800, 100000, nil, []EvidenceEndpoint{{URL: "https://mainnet.base.org", Operator: "base"}, {URL: "https://base-rpc.publicnode.com", Operator: "publicnode"}, {URL: "https://base.gateway.tenderly.co", Operator: "tenderly"}, {URL: "https://base.drpc.org", Operator: "drpc"}}, false},
	Network:          {Network, ChainID, Asset, TokenName, TokenVersion, 1800, 100000, nil, releaseEvidence, true},
	"eip155:5042002": {"eip155:5042002", 5042002, "0x3600000000000000000000000000000000000000", "USDC", "2", 1800, 100000, nil, []EvidenceEndpoint{{URL: "https://rpc.testnet.arc.io", Operator: "circle"}, {URL: "https://arc-testnet-rpc.publicnode.com", Operator: "publicnode"}}, true},
	"eip155:5042":    {"eip155:5042", 5042, "0x3600000000000000000000000000000000000000", "USDC", "2", 1800, 100000, nil, []EvidenceEndpoint{{URL: "https://rpc.mainnet.arc.io", Operator: "circle"}, {URL: "https://rpc.quicknode.mainnet.arc.io", Operator: "quicknode"}, {URL: "https://rpc.blockdaemon.mainnet.arc.io", Operator: "blockdaemon"}, {URL: "https://rpc.drpc.mainnet.arc.io", Operator: "drpc"}}, false},
	"eip155:46630":   {"eip155:46630", 46630, "0x7e955252e15c84f5768b83c41a71f9eba181802f", "Global Dollar", "1", 2400, 2000000, []string{"l1BlockNumber", "sendCount", "sendRoot"}, []EvidenceEndpoint{{URL: "https://rpc.testnet.chain.robinhood.com", Operator: "robinhood"}, {URL: "https://robinhood-sepolia-rpc.publicnode.com", Operator: "publicnode"}}, true},
	"eip155:4663":    {"eip155:4663", 4663, "0x5fc5360d0400a0fd4f2af552add042d716f1d168", "Global Dollar", "1", 2400, 2000000, []string{"l1BlockNumber", "sendCount", "sendRoot"}, []EvidenceEndpoint{{URL: "https://rpc.mainnet.chain.robinhood.com", Operator: "robinhood"}, {URL: "https://robinhood-rpc.publicnode.com", Operator: "publicnode"}, {URL: "https://robinhood.drpc.org", Operator: "drpc"}}, false},
}

func normalizedNetwork(network string) string {
	if network == "" {
		return Network
	}
	return network
}
func pins(network string) ChainPins { return chainPins[normalizedNetwork(network)] }
func testnet(network string) bool   { return pins(network).Testnet }
func supportedNetwork(network string) bool {
	p, ok := chainPins[normalizedNetwork(network)]
	return ok && len(p.Endpoints) >= 2
}
func ReleaseEvidenceFor(network string) (*Quorum, error) {
	if !supportedNetwork(network) {
		return nil, ErrInvalid
	}
	q, err := newQuorum(pins(network).Endpoints)
	if err != nil {
		return nil, err
	}
	for _, r := range q.rpcs {
		r.network = normalizedNetwork(network)
	}
	if os.Getenv("SECONDED_CHAIN_EVIDENCE") == "1" {
		endpoint := fmt.Sprintf("%s/v1/chain-evidence/%d", releaseAPIOrigin, pins(network).ChainID)
		h, err := pinnedHTTP(releaseAPIOrigin, nil)
		if err != nil {
			return nil, err
		}
		q.rpcs = append(q.rpcs, &RPC{network: normalizedNetwork(network), url: endpoint, http: h, server: true})
	}
	return q, nil
}

// LatestSnapshot is a display-only observation. It must never be used by the
// payment ledger or budget checks, which require Snapshot's finalized evidence.
func (q *Quorum) LatestSnapshot(ctx context.Context, payer string) (ChainSnapshot, error) {
	if len(q.rpcs) > 2 {
		return quorumPair(q, func(pair *Quorum) (ChainSnapshot, error) { return pair.LatestSnapshot(ctx, payer) })
	}
	if !q.publicPair() {
		return ChainSnapshot{}, errEvidenceNotConfigured
	}
	var low uint64
	for i, r := range q.rpcs {
		head, err := r.block(ctx, "latest")
		if err != nil {
			return ChainSnapshot{}, err
		}
		if i == 0 || head.Number < low {
			low = head.Number
		}
	}
	var first ChainSnapshot
	for i, r := range q.rpcs {
		snap, err := r.snapshotAt(ctx, payer, fmt.Sprintf("0x%x", low))
		if err != nil {
			return ChainSnapshot{}, err
		}
		if i == 0 {
			first = snap
		} else if snap != first {
			return ChainSnapshot{}, errEvidenceDisagreement
		}
	}
	return first, nil
}
