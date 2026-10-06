package client

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// A certificate always comes from a whole pair. Availability failures may
// select another independent pair; malformed or conflicting evidence may not.
// A server reader is useful only alongside a compiled public reader.
func quorumPair[T any](q *Quorum, read func(*Quorum) (T, error)) (T, error) {
	var zero T
	last := errEvidenceNotConfigured
	unavailable := map[*RPC]bool{}
	for i := 0; i < len(q.rpcs); i++ {
		for j := i + 1; j < len(q.rpcs); j++ {
			if unavailable[q.rpcs[i]] || unavailable[q.rpcs[j]] || (q.rpcs[i].server && q.rpcs[j].server) {
				continue
			}
			result, err := read(&Quorum{rpcs: []*RPC{q.rpcs[i], q.rpcs[j]}})
			if err == nil {
				return result, nil
			}
			if !errors.Is(err, ErrChainUnavailable) {
				return zero, err
			}
			var failed *rpcReadError
			if errors.As(err, &failed) && failed.reader != nil {
				unavailable[failed.reader] = true
			}
			last = err
		}
	}
	return zero, last
}

func (q *Quorum) publicPair() bool {
	return len(q.rpcs) >= 2 && !(len(q.rpcs) == 2 && q.rpcs[0].server && q.rpcs[1].server)
}

// Confirm that the previously certified anchor is still canonical and covered
// by today's recent balance. This does not certify a new finalized head.
func (q *Quorum) VerifyPaymentAnchor(ctx context.Context, anchor, snap ChainSnapshot) error {
	if !q.publicPair() {
		return errEvidenceNotConfigured
	}
	if len(q.rpcs) > 2 {
		_, err := quorumPair(q, func(pair *Quorum) (bool, error) {
			return true, pair.VerifyPaymentAnchor(ctx, anchor, snap)
		})
		return err
	}
	if !recentStateNetwork(snap.Network) || normalizedNetwork(anchor.Network) != normalizedNetwork(snap.Network) ||
		snap.StateNumber < anchor.Number || snap.StateTimestamp < anchor.Timestamp ||
		snap.Timestamp <= 0 || (!baseRecentNetwork(snap.Network) && snap.Timestamp < time.Now().Unix()-pins(snap.Network).FinalizedMaxAge) {
		return ErrInvalid
	}
	if err := q.recheckRecentSnapshot(ctx, snap); err != nil {
		return err
	}
	return q.readRecentReaders(func(_ int, r *RPC) error {
		if normalizedNetwork(r.network) != normalizedNetwork(snap.Network) {
			return ErrInvalid
		}
		block, err := r.block(ctx, fmt.Sprintf("0x%x", anchor.Number))
		if err != nil {
			return err
		}
		if block.Number != anchor.Number || block.Hash != anchor.Hash || block.Timestamp != anchor.Timestamp {
			return errEvidenceDisagreement
		}
		return nil
	})
}

// PreparePayment preserves the high-water anchor when a reader's finalized
// tag lags. No entry is released, settled, or aged using the regressed head.
// Recent balance covers the retained anchor; every open amount remains a debit.
func (l *Ledger) PreparePayment(ctx context.Context, chain ChainReader, snap ChainSnapshot) error {
	// Admission persists snap.Number as StartBlock. Leave half the history
	// window for finality to recover before certifying the payment's spend.
	if baseRecentNetwork(snap.Network) && snap.StateNumber > snap.Number &&
		snap.StateNumber-snap.Number > pins(snap.Network).HistoryBlocks/2 {
		return errors.New("budget_blocked_on_chain_evidence")
	}
	a := l.anchor(snap.Network)
	staleBase := baseRecentNetwork(snap.Network) && snap.Timestamp < time.Now().Unix()-pins(snap.Network).FinalizedMaxAge
	if staleBase || (a != nil && (snap.Number < a.Number || snap.Timestamp < a.Timestamp)) {
		// With no prior anchor, certify the agreed header without persisting it.
		// Stale finality must never advance budget time or release reservations.
		if a == nil {
			a = &snap
		}
		verifier, ok := chain.(interface {
			VerifyPaymentAnchor(context.Context, ChainSnapshot, ChainSnapshot) error
		})
		if !ok {
			return errors.New("budget_blocked_on_chain_evidence")
		}
		if err := verifier.VerifyPaymentAnchor(ctx, *a, snap); err != nil {
			return &budgetEvidenceError{stage: "reconciliation", cause: err}
		}
		// Recovery may have checkpointed a newer SPENT entry before it could
		// advance the shared anchor. Never reuse a balance predating that debit.
		for _, entry := range l.Entries {
			if normalizedNetwork(entry.Network) == normalizedNetwork(snap.Network) &&
				entry.State == "SPENT" && entry.SpentAt >= snap.StateTimestamp {
				return errors.New("budget_blocked_on_chain_evidence")
			}
		}
		return nil
	}
	return l.Reconcile(ctx, chain, snap)
}
