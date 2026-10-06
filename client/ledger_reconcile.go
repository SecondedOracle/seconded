package client

import (
	"context"
	"maps"
)

// Robinhood finality lags recent state, so repeatedly scanning every pending
// nonce makes foreground cost grow with the ledger. Check one whole entry per
// call and retain all unchecked reservations. Persisted terminal entries are
// skipped on the next poll; no new on-disk format or partial proof is needed.
func (l *Ledger) reconcileRecentEntry(ctx context.Context, chain ChainReader, snap ChainSnapshot) error {
	network := normalizedNetwork(snap.Network)
	anchor := Ledger{Version: l.Version, Anchor: l.Anchor, Anchors: maps.Clone(l.Anchors)}
	if err := anchor.reconcileAll(ctx, nil, snap); err != nil {
		return err
	}
	selected := -1
	for i, entry := range l.Entries {
		if entry.State == "OUTSTANDING" && normalizedNetwork(entry.Network) == network {
			selected = i
			break
		}
	}
	previousAnchor, previousAnchors := l.Anchor, l.Anchors
	var previous Entry
	if selected >= 0 {
		previous = l.Entries[selected]
		candidate := Ledger{Version: l.Version, Entries: []Entry{previous}, Anchor: l.Anchor, Anchors: maps.Clone(l.Anchors)}
		if err := candidate.reconcileAll(ctx, chain, snap); err != nil {
			return err
		}
		l.Entries[selected] = candidate.Entries[0]
	}
	// Finalized network time can age certified spending even on a bounded pass:
	// Totals still counts every RESERVED and OUTSTANDING amount in full, including
	// unchecked entries. Commit time and the checked entry together, or neither.
	l.Anchor, l.Anchors = anchor.Anchor, anchor.Anchors
	if l.files != nil {
		if err := l.Save(l.files); err != nil {
			if selected >= 0 {
				l.Entries[selected] = previous
			}
			l.Anchor, l.Anchors = previousAnchor, previousAnchors
			return err
		}
	}
	return nil
}

// reconcileEntries is for recovery while the caller holds the profile lock.
// A whole-entry quorum certificate is the durable checkpoint: no partial scan
// or single-reader result survives a failure. Interactive authorization still
// uses Reconcile; Robinhood's bounded path also preserves unchecked reservations.
func (l *Ledger) reconcileEntries(ctx context.Context, chain ChainReader, snap ChainSnapshot, files *Files) error {
	// Reuse Reconcile's snapshot/anchor guards without advancing the live anchor.
	checkpoint := func(entries []Entry) Ledger {
		return Ledger{Version: l.Version, Entries: entries, Anchor: l.Anchor, Anchors: maps.Clone(l.Anchors)}
	}
	anchor := checkpoint(nil)
	if err := anchor.reconcileAll(ctx, nil, snap); err != nil {
		return err
	}
	var evidenceErr error
	for i, entry := range l.Entries {
		if entry.State != "OUTSTANDING" || normalizedNetwork(entry.Network) != normalizedNetwork(snap.Network) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		candidate := checkpoint([]Entry{entry})
		if err := candidate.reconcileAll(ctx, chain, snap); err != nil {
			// A failed entry cannot roll back earlier durable certificates or
			// prevent another entry being checked while time remains.
			evidenceErr = err
			continue
		}
		if candidate.Entries[0].State == "OUTSTANDING" {
			continue
		}
		l.Entries[i] = candidate.Entries[0]
		if err := l.Save(files); err != nil {
			l.Entries[i] = entry
			return err
		}
	}
	if evidenceErr != nil {
		return evidenceErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Budget time may advance only after every outstanding entry was checked.
	previousAnchor, previousAnchors := l.Anchor, l.Anchors
	l.Anchor, l.Anchors = anchor.Anchor, anchor.Anchors
	if err := l.Save(files); err != nil {
		l.Anchor, l.Anchors = previousAnchor, previousAnchors
		return err
	}
	return nil
}
