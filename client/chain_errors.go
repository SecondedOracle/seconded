package client

import (
	"context"
	"errors"
)

// Keep the stable budget reason while retaining a typed, non-provider cause.
type budgetEvidenceError struct {
	stage string
	cause error
}

func (e *budgetEvidenceError) Error() string { return "budget_blocked_on_chain_evidence" }
func (e *budgetEvidenceError) Unwrap() error { return e.cause }

type rpcReadError struct {
	reader *RPC // Internal identity only; never formatted into an error.
	method string
	cause  error
}

func (e *rpcReadError) Error() string { return e.cause.Error() }
func (e *rpcReadError) Unwrap() error { return e.cause }

var errReaderTimeout error = readerTimeoutError{}

type readerTimeoutError struct{}

func (readerTimeoutError) Error() string { return "chain_reader_timeout" }
func (readerTimeoutError) Unwrap() error { return ErrChainUnavailable }

func evidenceFailureMessage(err error) string {
	var failure *budgetEvidenceError
	if !errors.As(err, &failure) {
		return ""
	}
	stage := "checking chain evidence"
	switch failure.stage {
	case "snapshot":
		stage = "reading the wallet snapshot"
	case "reconciliation":
		stage = "reconciling existing reservations"
	}
	var call *rpcReadError
	if errors.As(err, &call) {
		switch call.method {
		case "eth_chainId", "eth_getBlockByNumber", "eth_call", "eth_getLogs":
			stage += " (" + call.method + ")"
		}
	}
	cause := "Chain evidence could not be validated"
	switch {
	case errors.Is(err, ErrReaderRateLimited):
		cause = "A chain reader is rate-limited"
	case errors.Is(err, errReaderTimeout), errors.Is(err, context.DeadlineExceeded):
		cause = "Chain evidence timed out"
	case errors.Is(err, context.Canceled):
		cause = "Chain evidence reading was canceled"
	case errors.Is(err, errEvidenceDisagreement):
		cause = "The chain readers disagree"
	case errors.Is(err, ErrReaderHistoryUnavailable):
		cause = "A chain reader cannot serve the required history"
	case errors.Is(err, ErrChainUnavailable):
		cause = "A chain reader is unavailable"
	}
	return cause + " while " + stage + ". Existing funds remain reserved. Check seconded_wallet again shortly; if this persists, update the client or contact support."
}
