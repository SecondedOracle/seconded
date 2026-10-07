package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type limitUpdate struct {
	PerCheck    json.RawMessage `json:"per_check_usd,omitempty"`
	Hourly      json.RawMessage `json:"hourly_usd,omitempty"`
	Daily       json.RawMessage `json:"daily_usd,omitempty"`
	Outstanding json.RawMessage `json:"outstanding_usd,omitempty"`
	ValueGate   *bool           `json:"value_gate,omitempty"`
	Dedupe      *bool           `json:"dedupe,omitempty"`
	LoopBrake   *bool           `json:"loop_brake,omitempty"`
	Alerts      *bool           `json:"alerts,omitempty"`
	Frozen      *bool           `json:"frozen,omitempty"`
}

func publicLimits(s Settings) map[string]any {
	amount := func(v *int64) any {
		if v == nil {
			return nil
		}
		return Dollars(*v)
	}
	return map[string]any{
		"per_check_usd": amount(s.Limits.PerCheck), "hourly_usd": amount(s.Limits.Hour),
		"daily_usd": amount(s.Limits.Day), "outstanding_usd": amount(s.Limits.Outstanding),
		"value_gate": s.ValueGate, "dedupe": s.Dedupe, "loop_brake": s.LoopBrake,
		"alerts": s.Alerts, "frozen": s.Frozen, "max_authorization_usd": Dollars(MaxAuthorization),
		"chat_daily_ceiling_usd": Dollars(ChatDailyCeiling),
	}
}

func (g *Engine) Limits(args json.RawMessage, update bool) (any, error) {
	var patch limitUpdate
	if update {
		if err := DecodeStrict(args, &patch, MessageLimit); err != nil {
			return nil, err
		}
	} else {
		var empty struct{}
		if err := DecodeStrict(args, &empty, MessageLimit); err != nil {
			return nil, err
		}
	}
	unlock, err := g.Files.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	v, err := LoadVault(g.Store)
	if err != nil {
		return nil, err
	}
	if !update {
		return publicLimits(v.Settings), nil
	}
	next, err := patchSettings(v.Settings, patch)
	if err != nil {
		return nil, err
	}
	// Even reasserting a higher owner limit through a daily chat patch requires
	// terminal confirmation. An unrelated tightening may leave that limit alone.
	if len(patch.Daily) != 0 && next.Limits.Day != nil && *next.Limits.Day > ChatDailyCeiling {
		return nil, g.terminalPolicyError(patch)
	}
	if err = ChatPolicy(v.Settings, next); err != nil {
		return nil, g.terminalPolicyError(patch)
	}
	if err = g.savePolicyLocked(v, next, "chat"); err != nil {
		return nil, err
	}
	return publicLimits(next), nil
}

func patchSettings(current Settings, patch limitUpdate) (Settings, error) {
	next := current
	for _, field := range []struct {
		raw json.RawMessage
		dst **int64
	}{
		{patch.PerCheck, &next.Limits.PerCheck}, {patch.Hourly, &next.Limits.Hour},
		{patch.Daily, &next.Limits.Day}, {patch.Outstanding, &next.Limits.Outstanding},
	} {
		if len(field.raw) == 0 {
			continue
		}
		value := string(field.raw)
		if value == "null" {
			*field.dst = nil
			continue
		}
		if field.raw[0] == '"' {
			if json.Unmarshal(field.raw, &value) != nil {
				return Settings{}, ErrInvalid
			}
		}
		if value == "none" || value == "no_limit" {
			*field.dst = nil
			continue
		}
		n, err := USD(value)
		if err != nil {
			return Settings{}, err
		}
		*field.dst = &n
	}
	for _, field := range []struct {
		src *bool
		dst *bool
	}{
		{patch.ValueGate, &next.ValueGate}, {patch.Dedupe, &next.Dedupe},
		{patch.LoopBrake, &next.LoopBrake}, {patch.Alerts, &next.Alerts}, {patch.Frozen, &next.Frozen},
	} {
		if field.src != nil {
			*field.dst = *field.src
		}
	}
	if err := next.Validate(); err != nil {
		return Settings{}, err
	}
	return next, nil
}

var ErrTerminalPolicyRequired = errors.New("terminal_policy_required")

type terminalPolicyError struct{ command string }

func (e *terminalPolicyError) Error() string {
	return "This policy change requires the wallet owner. Run this in your terminal: " + e.command + ". Type the wallet address's last 6 characters when prompted. On Macs with Touch ID, owner changes also require biometrics. Otherwise an installed administrator approval key requires an offline signature. Without either, the six-character TTY confirmation does not stop an agent with shell access. Chat can only tighten limits or freeze. Raising or removing any limit, weakening a safety switch, unfreezing, and switching wallets require terminal confirmation."
}
func (e *terminalPolicyError) Unwrap() error { return ErrTerminalPolicyRequired }

func (g *Engine) terminalPolicyError(patch limitUpdate) error {
	raw, _ := json.Marshal(patch)
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	exe := "seconded-mcp"
	if i, err := readInstallation(g.Files); err == nil && i.Executable != "" {
		exe = i.Executable
	}
	return &terminalPolicyError{fmt.Sprintf("%s --profile %s limits --human --patch %s", quote(exe), quote(g.Files.Dir), quote(string(raw)))}
}

func limitToolSchema() any {
	properties := map[string]any{}
	for _, field := range []string{"per_check_usd", "hourly_usd", "daily_usd", "outstanding_usd"} {
		properties[field] = map[string]any{"anyOf": []any{
			map[string]any{"type": "number", "minimum": 0, "maximum": 999999999.999999},
			map[string]any{"type": "string", "pattern": "^(none|no_limit|(0|[1-9][0-9]{0,8})(\\.[0-9]{1,6})?)$"},
			map[string]any{"type": "null"},
		}, "description": "USD amount with up to six decimals; omit to leave unchanged. Chat may only lower an existing limit or add a limit. Raising or removing any limit requires owner-terminal confirmation. Chat may set daily limits only at or below $25, even when the previous daily limit was absent or higher. The compiled $2.50 authorization cap always applies."}
	}
	for _, field := range []string{"value_gate", "dedupe", "loop_brake", "alerts", "frozen"} {
		properties[field] = map[string]any{"type": "boolean"}
	}
	return schema(properties)
}
