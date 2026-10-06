package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Base funding and Robinhood's pruned readers need state before finality. It is used
// for balance/token identity and the monotone authorization bit; only finalized
// headers/events can settle a payment or advance budget time.
func recentStateNetwork(network string) bool {
	return network == "eip155:8453" || network == "eip155:84532" ||
		network == "eip155:4663" || network == "eip155:46630"
}

func baseRecentNetwork(network string) bool {
	return network == "eip155:8453" || network == "eip155:84532"
}

const recentStateDepth uint64 = 10
const recentStateMaxAge int64 = 15

func recentStateRule(network string) (depth uint64, maxAge int64) {
	if network == "eip155:8453" || network == "eip155:84532" {
		// Thirty 2-second blocks give Base about a minute of confirmation.
		// Allow 30 seconds beyond that depth for reader and request latency.
		return 30, 90
	}
	return recentStateDepth, recentStateMaxAge
}

func (q *Quorum) recentSnapshot(ctx context.Context, payer string) (ChainSnapshot, error) {
	network := q.rpcs[0].network
	depth, maxAge := recentStateRule(network)
	heads := make([]ChainSnapshot, len(q.rpcs))
	finals := make([]ChainSnapshot, len(q.rpcs))
	// Readers are independent; keep dependent reads ordered within each reader.
	if err := q.readRecentReaders(func(i int, r *RPC) error {
		if r.network != network {
			return errEvidenceDisagreement
		}
		if err := parallelReads(
			func() (err error) { finals[i], err = r.block(ctx, "finalized"); return },
			func() (err error) { heads[i], err = r.block(ctx, "latest"); return },
		); err != nil {
			return err
		}
		now := time.Now().Unix()
		if heads[i].Number < depth || heads[i].Number < finals[i].Number || heads[i].Timestamp < finals[i].Timestamp ||
			heads[i].Timestamp < now-recentStateMaxAge || heads[i].Timestamp > now+30 {
			return ErrInvalid
		}
		return nil
	}); err != nil {
		return ChainSnapshot{}, err
	}
	finalized, recent := finals[0].Number, heads[0].Number-depth
	for i := range heads {
		finalized = min(finalized, finals[i].Number)
		recent = min(recent, heads[i].Number-depth)
	}
	if recent < finalized {
		recent = finalized
	}
	snaps := make([]ChainSnapshot, len(q.rpcs))
	if err := q.readRecentReaders(func(i int, r *RPC) error {
		var f, state ChainSnapshot
		if err := parallelReads(
			func() (err error) { f, err = r.block(ctx, fmt.Sprintf("0x%x", finalized)); return },
			func() (err error) { state, err = r.snapshotAt(ctx, payer, fmt.Sprintf("0x%x", recent)); return },
		); err != nil {
			return err
		}
		now := time.Now().Unix()
		// Base admission uses fresh recent balance while finality lags. Ledger
		// reconciliation still enforces finalized age before freeing any funds.
		if f.Number != finalized || state.Number != recent || f.Timestamp > state.Timestamp ||
			f.Timestamp <= 0 || (!baseRecentNetwork(network) && f.Timestamp < now-pins(network).FinalizedMaxAge) || state.Timestamp < now-maxAge {
			return ErrInvalid
		}
		f.Network, f.Balance = network, state.Balance
		f.StateNumber, f.StateHash, f.StateTimestamp = state.Number, state.Hash, state.Timestamp
		snaps[i] = f
		return nil
	}); err != nil {
		return ChainSnapshot{}, err
	}
	first := snaps[0]
	for _, snap := range snaps[1:] {
		if snap != first {
			return ChainSnapshot{}, errEvidenceDisagreement
		}
	}
	if err := q.recheckRecentSnapshot(ctx, first); err != nil {
		return ChainSnapshot{}, err
	}
	return first, nil
}

func (q *Quorum) recheckRecentSnapshot(ctx context.Context, s ChainSnapshot) error {
	if !recentStateNetwork(s.Network) {
		return nil
	}
	_, maxAge := recentStateRule(s.Network)
	if !hexWord.MatchString(s.StateHash) || s.StateNumber < s.Number ||
		s.StateTimestamp < s.Timestamp || s.StateTimestamp < time.Now().Unix()-maxAge ||
		s.StateTimestamp > time.Now().Unix()+30 {
		return ErrInvalid
	}
	return q.readRecentReaders(func(_ int, r *RPC) error {
		var reads []func() error
		for _, expected := range []ChainSnapshot{
			{Number: s.Number, Hash: s.Hash, Timestamp: s.Timestamp},
			{Number: s.StateNumber, Hash: s.StateHash, Timestamp: s.StateTimestamp},
		} {
			reads = append(reads, func() error {
				actual, err := r.block(ctx, fmt.Sprintf("0x%x", expected.Number))
				if err != nil {
					return err
				}
				if actual.Number != expected.Number || actual.Hash != expected.Hash || actual.Timestamp != expected.Timestamp {
					return errEvidenceDisagreement
				}
				return nil
			})
		}
		return parallelReads(reads...)
	})
}

// Only independent reads belong in a group. Join all results before using any
// evidence; errors never leave a detached request or a partial certificate.
func parallelReads(reads ...func() error) error {
	errs := make([]error, len(reads))
	var group sync.WaitGroup
	for i, read := range reads {
		group.Add(1)
		go func() { defer group.Done(); errs[i] = read() }()
	}
	group.Wait()
	return evidenceReadFailure(errs)
}

func (r *RPC) recentLogs(ctx context.Context, from, to, chunk uint64, topics []any) ([]chainLog, error) {
	parts := make([][]chainLog, (to-from)/chunk+1)
	reads := make([]func() error, len(parts))
	for i := range parts {
		start := from + uint64(i)*chunk
		end := start + min(chunk-1, to-start)
		reads[i] = func() (err error) { parts[i], err = r.logs(ctx, start, end, topics); return }
	}
	if err := parallelReads(reads...); err != nil {
		return nil, err
	}
	var logs []chainLog
	for _, part := range parts {
		logs = append(logs, part...)
	}
	return logs, nil
}

// Join every reader before comparing or returning; no single-reader result can
// certify a snapshot, and a failed call leaves no background work behind.
func (q *Quorum) readRecentReaders(read func(int, *RPC) error) error {
	errs := make([]error, len(q.rpcs))
	var readers sync.WaitGroup
	for i, r := range q.rpcs {
		readers.Add(1)
		go func() {
			defer readers.Done()
			errs[i] = read(i, r)
		}()
	}
	readers.Wait()
	return evidenceReadFailure(errs)
}

// A simultaneous capability failure must never hide malformed evidence.
func evidenceReadFailure(errs []error) error {
	for _, err := range errs {
		if err != nil && !errors.Is(err, ErrChainUnavailable) {
			return err
		}
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
