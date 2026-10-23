package client

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
)

// Only fixed labels and a reader hostname cross this boundary. Never format an
// error, URL, request, receipt, or ledger entry: those can contain credentials.
func debugSettlement(r *RPC, method string, err error, decision string) {
	if os.Getenv("SECONDED_DEBUG_SETTLEMENT") != "1" {
		return
	}
	host := "none"
	var call *rpcReadError
	if r == nil && errors.As(err, &call) {
		r = call.reader
		method = call.method
	}
	if r != nil {
		if u, e := url.Parse(r.url); e == nil {
			host = u.Hostname()
		}
	}
	if !oneOf(method, "eth_chainId", "eth_getBlockByNumber", "eth_call", "eth_getLogs", "eth_getTransactionReceipt", "snapshot", "reconcile") {
		method = "other"
	}
	if !oneOf(decision, "rpc", "certificate", "retain", "accept") {
		decision = "other"
	}
	fmt.Fprintf(os.Stderr, "settlement reader=%q method=%s error=%s decision=%s\n", host, method, settlementErrorClass(err), decision)
}

func settlementErrorClass(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, ErrReaderRateLimited):
		return "rate_limited"
	case errors.Is(err, errReaderTimeout), errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, ErrReaderHistoryUnavailable):
		return "history_unavailable"
	case errors.Is(err, ErrReaderCapabilityUnavailable):
		return "capability_unavailable"
	case errors.Is(err, ErrChainUnavailable):
		return "unavailable"
	case errors.Is(err, errEvidenceDisagreement):
		return "disagreement"
	case errors.Is(err, ErrInvalid):
		return "invalid"
	default:
		return "unvalidated"
	}
}

// Recovery reasons are fixed labels only. IDs, receipt fields, request bodies,
// signatures, filesystem paths and arbitrary errors never reach stderr.
func debugRecovery(reason string, err error) {
	if os.Getenv("SECONDED_DEBUG_SETTLEMENT") != "1" {
		return
	}
	switch reason {
	case "recovery_configuration_failed", "receipt_export_local",
		"receipt_pair_unconfigured", "receipt_pair_disagreement", "receipt_reader_failed",
		"receipt_snapshot_changed", "receipt_block_recheck_failed", "receipt_pair_verified",
		"api_unconfigured", "certificate_retained", "chain_unconfigured", "collection_accepted",
		"collection_backoff", "collection_begin", "collection_failed", "collection_not_needed",
		"collection_same_revision", "entry_credit", "entry_not_outstanding", "entry_not_selected",
		"entry_outside_listing", "evidence_failed", "evidence_invalid", "evidence_outstanding",
		"final_network_ineligible", "final_receipt_unavailable", "invalid_check_id", "ledger_entry_ineligible",
		"ledger_read_failed", "ledger_save_failed", "listing_limit", "locator_billing_invalid",
		"locator_network_ineligible", "locator_receipt_missing", "locator_receipt_not_final", "locator_verification_failed",
		"locator_verified", "lock_acquired", "lock_failed", "lock_wait",
		"network_reader_failed", "receipt_identity_invalid", "receipts_begin", "receipts_context_done",
		"receipts_saved", "reconcile_applied", "reconcile_begin", "reconcile_context_done",
		"recovery_signer_failed", "result_projected", "snapshot_failed", "snapshot_network_mismatch",
		"snapshot_invalid", "anchor_regression", "unknown_check":
	default:
		reason = "other"
	}
	fmt.Fprintf(os.Stderr, "settlement recovery=%s error=%s\n", reason, settlementErrorClass(err))
}
