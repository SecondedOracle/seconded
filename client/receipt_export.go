package client

import (
	"encoding/json"
	"errors"
	"time"
)

// ExportLocalReceipt returns only the saved signed envelope, signature and key ID.
// It verifies the local purchase binding without opening a wallet or recovering
// from the service. Signed billing is not independent proof of chain finality.
func ExportLocalReceipt(files *Files, checkID string) (json.RawMessage, error) {
	if !hexID.MatchString(checkID) {
		return nil, ErrInvalid
	}
	unlock, err := files.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	ledger, err := ReadLedger(files)
	if err != nil {
		return nil, err
	}
	entry := ledger.Find(checkID)
	if entry == nil {
		return nil, ErrNotFound
	}
	if entry.Receipt == nil {
		return nil, errors.New("receipt_unavailable")
	}
	if err := (ReceiptVerifier{labels: releaseLabels}).VerifyStored(*entry.Receipt, *entry, time.Now()); err != nil {
		return nil, errors.New("receipt_verification_failed")
	}
	return json.Marshal(entry.Receipt)
}

// Keep first-call wallet notices outside the standalone export document.
func receiptExportRequested(name string, args json.RawMessage) bool {
	var input struct {
		Export bool `json:"export"`
	}
	return name == "seconded_receipt" && json.Unmarshal(args, &input) == nil && input.Export
}
