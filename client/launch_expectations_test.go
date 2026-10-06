package client

import (
	"strings"
	"testing"
)

func TestLocalPrivacyToolExpectations(t *testing.T) {
	rows := privacyGuides()
	if len(rows) == 0 {
		t.Fatal("no privacy descriptions checked")
	}
	for _, row := range rows {
		if !strings.Contains(row.Description, "No anonymity or legal-compliance guarantee.") {
			t.Fatal(row.Tool)
		}
	}
}
