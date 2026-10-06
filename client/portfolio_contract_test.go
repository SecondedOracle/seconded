package client

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestPortfolioContractRejectsUnsupportedVocabulary(t *testing.T) {
	// The complete current schema remains the positive control.
	contract := loadPortfolioContract(portfolioSchemaJSON)
	if contract.Request == nil || contract.Report == nil {
		t.Fatal("missing supported contract")
	}
	for _, location := range []string{"request", "report", "properties", "items", "anyOf"} {
		for _, fragment := range []string{`{"type":"number"}`, `{"type":"null"}`, `{"type":"future"}`, `{"not":{}}`} {
			t.Run(location+"/"+fragment, func(t *testing.T) {
				var schema map[string]json.RawMessage
				if err := json.Unmarshal(portfolioSchemaJSON, &schema); err != nil {
					t.Fatal(err)
				}
				field, shape := "request", fragment
				switch location {
				case "report":
					field = "report"
				case "properties":
					shape = `{"type":"object","properties":{"nested":` + fragment + `}}`
				case "items":
					shape = `{"type":"array","items":` + fragment + `}`
				case "anyOf":
					shape = `{"anyOf":[` + fragment + `]}`
				}
				schema[field] = json.RawMessage(shape)
				data, err := json.Marshal(schema)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					failure := recover()
					if failure == nil {
						t.Fatal("unsupported schema accepted")
					}
					want := "unsupported Portfolio schema type"
					if strings.Contains(fragment, `"not"`) {
						want = `json: unknown field "not"`
					}
					if !strings.Contains(fmt.Sprint(failure), want) {
						t.Fatalf("unexpected panic: %v", failure)
					}
				}()
				loadPortfolioContract(data)
			})
		}
	}
}
