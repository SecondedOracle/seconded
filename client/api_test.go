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
