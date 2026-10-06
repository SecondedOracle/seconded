package client

import (
	"context"
	"errors"
	"sync"
)

type releaseChainReader struct{ *Quorum }

func (r releaseChainReader) ForNetwork(network string) (ChainReader, error) {
	if network == Network {
		return r.Quorum, nil
	}
	return ReleaseEvidenceFor(network)
}

func walletReader(reader ChainReader, network string) (ChainReader, error) {
	if router, ok := reader.(interface {
		ForNetwork(string) (ChainReader, error)
	}); ok {
		return router.ForNetwork(network)
	}
	if reader != nil && network == Network {
		return reader, nil
	}
	return nil, errors.New("network_reader_unavailable")
}

const FundingInstructions = "Use the displayed address on the network your check names. Base and Arc use USDC; Robinhood Chain uses USDG. Checks default to Base mainnet, which needs USDC on Base. Base funding normally becomes available after about a minute when both readers agree; final payment confirmation is separate. Testnet funds do not fund mainnet checks, and mainnet funds do not fund testnet checks. Keep only what checks need in this dedicated wallet."

type WalletBalance struct {
	Network  string  `json:"network"`
	Name     string  `json:"name"`
	Asset    string  `json:"asset"`
	Token    string  `json:"token"`
	Testnet  bool    `json:"testnet"`
	Balance  *string `json:"balance_usd"`
	Evidence string  `json:"chain_evidence"`
}

func (g *Engine) walletBalances(ctx context.Context, address string) ([]WalletBalance, *ChainSnapshot) {
	networks := []struct{ id, name, token string }{
		{"eip155:8453", "Base", "USDC"}, {"eip155:5042", "Arc", "USDC"},
		{"eip155:4663", "Robinhood Chain", "USDG"}, {Network, "Base Sepolia", "USDC"},
		{"eip155:5042002", "Arc testnet", "USDC"}, {"eip155:46630", "Robinhood testnet", "USDG"},
	}
	out := make([]WalletBalance, len(networks))
	var defaultSnapshot *ChainSnapshot
	var wg sync.WaitGroup
	for i, network := range networks {
		out[i] = WalletBalance{Network: network.id, Name: network.name, Asset: pins(network.id).Asset, Token: network.token, Testnet: testnet(network.id), Evidence: "unavailable"}
		if g.Chain == nil {
			continue
		}
		wg.Add(1)
		go func(i int, network string) {
			defer wg.Done()
			chain, err := walletReader(g.Chain, network)
			if err != nil {
				return
			}
			snap, err := chain.Snapshot(ctx, address)
			if err != nil || normalizedNetwork(snap.Network) != network || snap.Balance < 0 {
				return
			}
			balance := Dollars(snap.Balance)
			out[i].Balance, out[i].Evidence = &balance, "finalized"
			if snap.StateHash != "" {
				out[i].Evidence = "recent_agreement"
			}
			if network == DefaultPaymentNetwork {
				defaultSnapshot = &snap
			}
		}(i, network.id)
	}
	wg.Wait()
	return out, defaultSnapshot
}
