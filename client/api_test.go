package client

import (
	"errors"
	"testing"
)

func TestPublicAPIErrorPaymentDoorClosed(t *testing.T) {
	for _, body := range []string{
		`{"error":"cell_unavailable","reason":"no_open_cell"}`,
		`{"error":"cell_unavailable"}`,
		`{"error":"no_open_cell"}`,
		`{"error":"store_unavailable","reason":"cell_unavailable"}`,
		`{"error":"store_unavailable","reason":"no_open_cell"}`,
	} {
		t.Run(body, func(t *testing.T) {
			err := publicAPIError(503, []byte(body))
			var closed *PaymentDoorClosedError
			if !errors.As(err, &closed) || errors.Is(err, ErrInvalid) {
				t.Fatalf("expected typed payment door error, got %T: %v", err, err)
			}
			if err.Error() != "payment door closed for this network" {
				t.Fatal(err)
			}
		})
	}
}

func TestPublicAPIErrorPreservesValidation(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{200, `{"error":"cell_unavailable"}`, ErrInvalid.Error()},
		{503, `{"error":"cell_unavailable","extra":"untrusted"}`, ErrInvalid.Error()},
		{503, `{"error":"cell_unavailable","reason":17}`, ErrInvalid.Error()},
		{503, `{"error":"unknown"}`, ErrInvalid.Error()},
		{503, `{"error":"store_unavailable"}`, "store_unavailable"},
		{422, `{"error":"invalid_input","reason":"missing_field"}`, "missing_field"},
	} {
		err := publicAPIError(tc.status, []byte(tc.body))
		var closed *PaymentDoorClosedError
		if err == nil || err.Error() != tc.want || errors.As(err, &closed) {
			t.Fatalf("status %d body %s: %v", tc.status, tc.body, err)
		}
	}
}

func TestPublicAPICreditStatusGate(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		want   string
	}{
		{503, "burn_credits_unavailable", "burn_credits_unavailable"},
		{503, "capacity_limited", "capacity_limited"},
		{503, "admission_hold", "admission_hold"},
		{401, "credit_authorization_required", "credit_authorization_required"},
		{402, "insufficient_credit", "insufficient_credit"},
		{408, "request_timeout", "request_timeout"},
		{423, "wallet_frozen", "wallet_frozen"},
		{429, "fair_use_limited", "fair_use_limited"},
		{422, "pool_liquidity_below_minimum", "pool_liquidity_below_minimum"},
		{200, "pool_liquidity_below_minimum", "invalid_response"},
		{503, "pool_liquidity_below_minimum", "invalid_response"},
	} {
		err := publicAPIError(tc.status, []byte(`{"error":"`+tc.code+`"}`))
		if safeError(err) != tc.want {
			t.Fatalf("%d %s: %v", tc.status, tc.code, err)
		}
	}
	for _, status := range []int{401, 402, 408, 409, 413, 422, 423, 426, 429, 500, 503} {
		if err := publicAPIError(status, []byte(`{"error":"injected_unknown_refusal"}`)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("unknown code escaped at %d: %v", status, err)
		}
	}
}
