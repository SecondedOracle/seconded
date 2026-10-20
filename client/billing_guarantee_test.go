package client

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBillingGuaranteeMatchesCatalogUsage(t *testing.T) {
	var guide struct {
		Usage []string `json:"usage"`
	}
	if err := json.Unmarshal(productGuideJSON, &guide); err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, line := range guide.Usage {
		if strings.HasPrefix(line, "Standard checks charge") {
			found++
			if line != billingGuarantee {
				t.Fatalf("initialize billing differs from catalog usage: %q != %q", billingGuarantee, line)
			}
		}
	}
	if found != 1 {
		t.Fatalf("want one catalog billing guarantee, got %d", found)
	}
}
