package client

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed privacy-tools.json
var privacyGuideJSON []byte

type privacyGuide struct {
	ID          string `json:"id"`
	Tool        string `json:"tool"`
	Status      string `json:"status"`
	Price       string `json:"price_usd"`
	Execution   string `json:"execution"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

var privacyGuideCache = sync.OnceValue(decodePrivacyGuides)

func decodePrivacyGuides() []privacyGuide {
	var result []privacyGuide
	decoder := json.NewDecoder(bytes.NewReader(privacyGuideJSON))
	decoder.UseNumber()
	if decoder.Decode(&result) != nil {
		panic("invalid privacy catalogue")
	}
	return result
}

func privacyGuides() []privacyGuide {
	rows := append([]privacyGuide(nil), privacyGuideCache()...)
	for i := range rows {
		rows[i].Status = toolAvailability(rows[i].Status)
		rows[i].Description += " No anonymity or legal-compliance guarantee."
		rows[i].InputSchema = privacyClone(rows[i].InputSchema)
	}
	return rows
}

func privacyTool(name string) (privacyGuide, bool) {
	for _, p := range privacyGuides() {
		if p.Tool == name {
			return p, true
		}
	}
	return privacyGuide{}, false
}

func privacyTools() []Tool {
	var out []Tool
	for _, p := range privacyGuides() {
		if p.Status == "available" {
			out = append(out, Tool{p.Tool, p.Description, p.InputSchema,
				map[string]bool{"readOnlyHint": p.ID == "privacy_shielded_route", "destructiveHint": false,
					"idempotentHint": p.ID == "privacy_shielded_route", "openWorldHint": p.ID == "privacy_pool_check"}})
		}
	}
	return out
}

// These calls intentionally bypass wallet setup, Refresh and payment bookkeeping.
func callPrivacy(ctx context.Context, g *Engine, name string, args json.RawMessage) (any, error) {
	p, found := privacyTool(name)
	if !found || p.Status != "available" {
		return map[string]any{"status": "unavailable", "reason": "privacy_tool_in_testing", "charged": "no"}, nil
	}
	var input map[string]any
	if DecodeStrict(args, &input, 30*1024) != nil || input == nil {
		return map[string]any{"status": "rejected", "reason": "invalid_privacy_request", "charged": "no"}, nil
	}
	if p.ID == "privacy_pool_check" && input["evidence_source"] == "hosted_neutral" {
		return hostedPrivacyPool(ctx, g, input), nil
	}
	raw, err := json.Marshal(map[string]any{"tool": p.ID, "input": input})
	if err != nil {
		return nil, ErrInvalid
	}
	process, ok := ctx.Value(privacySessionKey{}).(*privacyProcess)
	if !ok || process == nil {
		return privacyMap{"status": "unknown", "reason": "privacy_session_required", "charged": "no"}, nil
	}
	return process.call(ctx, raw), nil
}

type privacySessionKey struct{}

func (p *privacyProcess) lock(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	p.once.Do(func() { p.gate = make(chan struct{}, 1) })
	select {
	case p.gate <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}
func (p *privacyProcess) stop() {
	if p.native != nil {
		p.native.close()
		p.native = nil
	}
	p.stopCompanion()
	p.closed = true
}
func (p *privacyProcess) close() {
	if p.lock(context.Background()) {
		defer func() { <-p.gate }()
		p.stop()
	}
}
func isPrivacyName(name string) bool {
	// This paid server check is distinct from local private-receive preparation.
	if name == "seconded_private_receive_scan" {
		return false
	}
	return strings.HasPrefix(name, "seconded_privacy_") || strings.HasPrefix(name, "seconded_private_")
}

// Only the explicit chain-only schema may reach the hosted evidence endpoint.
func hostedPrivacyPool(ctx context.Context, g *Engine, input map[string]any) any {
	unavailable := map[string]any{"status": "unknown", "reason": "privacy_evidence_unavailable", "charged": "no"}
	chain, ok := input["chain_id"].(json.Number)
	n, err := chain.Int64()
	if !ok || err != nil || n <= 0 || n > 9007199254740991 || len(input) != 2 {
		return map[string]any{"status": "rejected", "reason": "invalid_privacy_request", "charged": "no"}
	}
	var api *API
	if g != nil {
		api = g.API
	}
	if api == nil {
		api, err = ReleaseAPI()
		if err != nil {
			return unavailable
		}
	}
	status, raw, _, err := api.request(ctx, "POST", "/v1/privacy/pool-check", map[string]any{"chain_id": chain}, nil)
	if err != nil || status != 200 {
		return unavailable
	}
	result := privacyHostedResponse(raw, chain)
	if result == nil {
		return unavailable
	}
	return result
}
