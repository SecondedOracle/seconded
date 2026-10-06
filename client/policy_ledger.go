package client

import "errors"

// Policy accounting is profile-wide even after an owner selects a retained
// wallet. Balance reservations and reconciliation remain wallet-specific.
func (g *Engine) policyLedger(active Ledger) (Ledger, error) {
	i, err := readInstallation(g.Files)
	if errors.Is(err, ErrNotFound) {
		return active, nil
	}
	if err != nil {
		return Ledger{}, err
	}
	name, err := ledgerFilename(g.Files)
	if err != nil {
		return Ledger{}, err
	}
	result := active
	result.Entries = append([]Entry{}, active.Entries...)
	result.Anchors = map[string]ChainSnapshot{}
	for network, anchor := range active.Anchors {
		result.Anchors[network] = anchor
	}
	candidates := map[string]string{"ledger.json": i.NewerAddress}
	if candidates["ledger.json"] == "" {
		candidates["ledger.json"] = i.Address
	}
	if i.EarlierAddress != "" {
		candidates["ledger-"+i.EarlierAddress+".json"] = i.EarlierAddress
	}
	for filename, payer := range candidates {
		if filename == name {
			continue
		}
		retained, err := readLedgerFile(g.Files, filename)
		// An earlier address may be discovered before a private ledger is created.
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return Ledger{}, err
		}
		for _, entry := range retained.Entries {
			if entry.Payer != payer {
				return Ledger{}, ErrStorage
			}
		}
		result.Entries = append(result.Entries, retained.Entries...)
		// Use the older verified time so a wallet switch cannot age out charges.
		for network := range chainPins {
			old := retained.anchor(network)
			current := result.anchor(network)
			if old == nil || current == nil {
				result.Anchors[network] = ChainSnapshot{}
				continue
			}
			if old.Timestamp < current.Timestamp {
				result.Anchors[network] = *old
			}
		}
	}
	return result, nil
}
