package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

type Entry struct {
	CheckedSubject          *CheckedSubject `json:"checked_subject,omitempty"`
	PortfolioScopeSHA256    string          `json:"portfolio_scope_sha256,omitempty"`
	ComparisonRequestSHA256 string          `json:"comparison_request_sha256,omitempty"`
	RegistryRequestSHA256   string          `json:"registry_request_sha256,omitempty"`
	PaymentDoor             string          `json:"payment_door,omitempty"`
	Recovery                *RecoveryRecord `json:"recovery,omitempty"`
	CommitmentSalt          string          `json:"commitment_salt,omitempty"`
	RecoveryRequired        bool            `json:"recovery_required,omitempty"`
	Network                 string          `json:"network,omitempty"`
	CheckID                 string          `json:"check_id"`
	Product                 string          `json:"product"`
	Tier                    string          `json:"tier"`
	InputDigest             string          `json:"input_digest"`
	Commitment              string          `json:"commitment"`
	Nonce                   string          `json:"nonce"`
	Payer                   string          `json:"payer"`
	Amount                  int64           `json:"amount"`
	State                   string          `json:"state"`
	SignedAt                int64           `json:"signed_at"`
	ValidBefore             int64           `json:"valid_before"`
	StartBlock              uint64          `json:"start_block"`
	SpentAt                 int64           `json:"spent_at"`
	Tx                      string          `json:"tx"`
	Payload                 string          `json:"payload"`
	Receipt                 *SignedReceipt  `json:"receipt,omitempty"`
}
type Ledger struct {
	// A loaded ledger can checkpoint a complete Robinhood certificate while its
	// caller holds the profile lock. This handle is never serialized.
	files   *Files
	Anchors map[string]ChainSnapshot `json:"anchors,omitempty"`
	Version int                      `json:"version"`
	// Early builds wrote null after reconciling an empty ledger; keep those profiles readable.
	Entries []Entry        `json:"entries" seconded:"nullable"`
	Anchor  *ChainSnapshot `json:"anchor,omitempty"`
}

func (l Ledger) MarshalJSON() ([]byte, error) {
	type wire Ledger
	if l.Entries == nil {
		l.Entries = []Entry{}
	}
	return json.Marshal(wire(l))
}

func ReadLedger(f *Files) (Ledger, error) {
	var l Ledger
	name, e := ledgerFilename(f)
	if e != nil {
		return l, e
	}
	l, e = readLedgerFile(f, name)
	if e == nil {
		// Only the active wallet may checkpoint implicitly during reconciliation.
		l.files = f
	}
	return l, e
}

func readLedgerFile(f *Files, name string) (Ledger, error) {
	var l Ledger
	b, e := f.Read(name)
	if e != nil {
		return l, e
	}
	if DecodeStrict(b, &l, 32<<20) != nil || l.Version != 1 || len(l.Entries) > 50000 {
		return l, ErrStorage
	}
	if l.Entries == nil {
		l.Entries = []Entry{}
	}
	if a := l.Anchor; a != nil && (!hexWord.MatchString(a.Hash) || a.Timestamp <= 0) {
		return l, ErrStorage
	}
	for network, anchor := range l.Anchors {
		if !supportedNetwork(network) || normalizedNetwork(anchor.Network) != network || !hexWord.MatchString(anchor.Hash) || anchor.Timestamp <= 0 {
			return l, ErrStorage
		}
	}
	seen := map[string]bool{}
	for _, x := range l.Entries {
		if !supportedNetwork(x.Network) || !hexID.MatchString(x.CheckID) || !hexWord.MatchString(x.Nonce) || seen[x.Nonce] || x.Amount <= 0 || x.Amount > MaxAuthorization || !addressPattern.MatchString(x.Payer) || !productOK(x.Product) || x.ValidBefore <= 0 || x.SignedAt <= 0 {
			return l, ErrStorage
		}
		if x.CheckedSubject != nil && (x.CheckedSubject.InputSHA256 != x.InputDigest || x.CheckedSubject.Identifiers == nil) {
			return l, ErrStorage
		}
		if !validComparisonEntry(x) || !validPortfolioEntry(x) {
			return l, ErrStorage
		}
		if x.CommitmentSalt != "" && !hexDigest.MatchString(x.CommitmentSalt) {
			return l, ErrStorage
		}
		if !oneOf(x.PaymentDoor, "", "standard") || (x.PaymentDoor == "standard" && x.State != "RESERVED" && x.Recovery == nil) {
			return l, ErrStorage
		}
		if x.Recovery != nil && ((x.Receipt == nil) != (x.Recovery.ServerCheckID == "") || x.Recovery.Validate(x) != nil) {
			return l, ErrStorage
		}
		seen[x.Nonce] = true
		switch x.State {
		case "RESERVED", "OUTSTANDING", "SPENT", "RELEASED":
		default:
			return l, ErrStorage
		}
		if x.State == "SPENT" && (x.SpentAt <= 0 || !hexWord.MatchString(x.Tx)) {
			return l, ErrStorage
		}
	}
	return l, nil
}
func (l *Ledger) Save(f *Files) error {
	b, e := json.Marshal(l)
	if e != nil {
		return ErrStorage
	}
	name, e := ledgerFilename(f)
	if e != nil {
		return e
	}
	return f.Write(name, b)
}
func (l *Ledger) Find(id string) *Entry {
	for i := range l.Entries {
		if l.Entries[i].CheckID == id {
			return &l.Entries[i]
		}
	}
	return nil
}
func (l *Ledger) Duplicate(product, digest string, now int64, dedupe bool) *Entry {
	for i := len(l.Entries) - 1; i >= 0; i-- {
		e := &l.Entries[i]
		if e.Product != product || e.InputDigest != digest {
			continue
		}
		if e.RecoveryRequired || e.State == "RESERVED" || e.State == "OUTSTANDING" {
			return e
		}
		if dedupe && e.Receipt != nil && e.SignedAt >= now-300 {
			return e
		}
	}
	return nil
}

// ReservedForNetwork counts funds that cannot yet be reused on this network.
// Legacy entries without a network belong to the default network.
func (l *Ledger) ReservedForNetwork(network string) (reserved int64) {
	network = normalizedNetwork(network)
	for _, e := range l.Entries {
		if normalizedNetwork(e.Network) == network && (e.State == "RESERVED" || e.State == "OUTSTANDING") {
			reserved += e.Amount
		}
	}
	return
}

// Totals remains global across networks for spending limits and their alerts.
func (l *Ledger) Totals(now int64) (hour, day, open int64) {
	// Only a persisted finalized anchor can age a payment out, even in offline UI.
	if l.Anchor == nil {
		now = 0
	} else {
		now = l.Anchor.Timestamp
	}
	for _, e := range l.Entries {
		anchor := l.anchor(e.Network)
		now = 0
		if anchor != nil {
			now = anchor.Timestamp
		}
		switch e.State {
		case "RESERVED", "OUTSTANDING":
			open += e.Amount
			hour += e.Amount
			day += e.Amount
		case "SPENT":
			if e.SpentAt > now-3600 {
				hour += e.Amount
			}
			if e.SpentAt > now-86400 {
				day += e.Amount
			}
		}
	}
	return
}
func (l *Ledger) Budget(s Settings, amount, now int64) error {
	if s.Frozen {
		return errors.New("frozen_local")
	}
	if amount <= 0 || amount > MaxAuthorization {
		return errors.New("policy_mismatch")
	}
	hour, day, open := l.Totals(now)
	for _, v := range []struct {
		limit  *int64
		used   int64
		reason string
	}{{s.Limits.PerCheck, 0, "limit_per_check"}, {s.Limits.Hour, hour, "limit_hour"}, {s.Limits.Day, day, "limit_day"}, {s.Limits.Outstanding, open, "limit_outstanding"}} {
		if v.limit != nil && (v.used > *v.limit || amount > *v.limit-v.used) {
			return errors.New(v.reason)
		}
	}
	if s.LoopBrake {
		n := 0
		for _, e := range l.Entries {
			if e.SignedAt >= time.Now().Unix()-60 {
				n++
			}
		}
		if n >= 30 {
			return errors.New("loop_brake")
		}
	}
	return nil
}
func (l *Ledger) Alerts(s Settings, now int64) []string {
	out := []string{}
	if !s.Alerts {
		return out
	}
	h, d, o := l.Totals(now)
	for _, v := range []struct {
		name  string
		limit *int64
		used  int64
	}{{"hour", s.Limits.Hour, h}, {"day", s.Limits.Day, d}, {"outstanding", s.Limits.Outstanding, o}} {
		if v.limit == nil {
			continue
		}
		for _, pct := range []int64{50, 80, 100} {
			if v.used >= (*v.limit*pct+99)/100 {
				out = append(out, v.name+"_"+map[int64]string{50: "50", 80: "80", 100: "100"}[pct])
			}
		}
	}
	return out
}

type ChainSnapshot struct {
	StateNumber    uint64 `json:",omitempty"`
	StateHash      string `json:",omitempty"`
	StateTimestamp int64  `json:",omitempty"`
	Network        string `json:",omitempty"`
	Number         uint64
	Hash           string
	Timestamp      int64
	Balance        int64
}
type ChainEvidence struct {
	// Compare the observed bit too: different recent states may both be outstanding.
	authorizationUsed bool
	State             string
	SpentAt           int64
	Tx                string
}
type ChainReader interface {
	Snapshot(context.Context, string) (ChainSnapshot, error)
	Evidence(context.Context, Entry, ChainSnapshot) (ChainEvidence, error)
}

func (l *Ledger) Reconcile(ctx context.Context, chain ChainReader, snap ChainSnapshot) error {
	if recentStateNetwork(snap.Network) {
		return l.reconcileRecentEntry(ctx, chain, snap)
	}
	return l.reconcileAll(ctx, chain, snap)
}

func (l *Ledger) reconcileAll(ctx context.Context, chain ChainReader, snap ChainSnapshot) error {
	if snap.Timestamp <= 0 || !hexWord.MatchString(snap.Hash) || snap.Timestamp > time.Now().Unix()+30 || snap.Timestamp < time.Now().Unix()-pins(snap.Network).FinalizedMaxAge {
		return errors.New("budget_blocked_on_chain_evidence")
	}
	if a := l.anchor(snap.Network); a != nil && (snap.Number < a.Number || snap.Timestamp < a.Timestamp || (snap.Number == a.Number && (snap.Hash != a.Hash || snap.Timestamp != a.Timestamp)) || (snap.Number > a.Number && snap.Timestamp < a.Timestamp)) {
		return errors.New("budget_blocked_on_chain_evidence")
	}
	// Stage all evidence so a later RPC failure cannot partially advance the ledger.
	staged := make([]Entry, len(l.Entries))
	copy(staged, l.Entries)
	original := l.Entries
	l.Entries = staged
	committed := false
	defer func() {
		if !committed {
			l.Entries = original
		}
	}()
	for i := range l.Entries {
		e := &l.Entries[i]
		if e.State != "OUTSTANDING" || normalizedNetwork(e.Network) != normalizedNetwork(snap.Network) {
			continue
		}
		proof, err := chain.Evidence(ctx, *e, snap)
		if err != nil {
			return &budgetEvidenceError{stage: "reconciliation", cause: err}
		}
		switch proof.State {
		case "SPENT":
			if proof.SpentAt <= 0 || proof.SpentAt > snap.Timestamp || !hexWord.MatchString(proof.Tx) {
				return ErrInvalid
			}
			e.State = "SPENT"
			e.SpentAt = proof.SpentAt
			e.Tx = proof.Tx
			e.Payload = ""
		case "RELEASED":
			e.State = "RELEASED"
			e.Payload = ""
		case "OUTSTANDING":
		default:
			return ErrInvalid
		}
	}
	if l.Anchors == nil {
		l.Anchors = map[string]ChainSnapshot{}
	}
	l.Anchors[normalizedNetwork(snap.Network)] = snap
	if normalizedNetwork(snap.Network) == Network {
		l.Anchor = &snap
	}
	committed = true
	return nil
}
func budgetTime(s ChainSnapshot) int64 {
	now := time.Now().Unix()
	if s.Timestamp < now {
		return s.Timestamp
	}
	return now
}

func (l *Ledger) anchor(network string) *ChainSnapshot {
	if a, ok := l.Anchors[normalizedNetwork(network)]; ok {
		return &a
	}
	if normalizedNetwork(network) == Network {
		return l.Anchor
	}
	return nil
}

// Older profiles use ledger.json; restored wallets have independent payment state.
func ledgerFilename(f *Files) (string, error) {
	i, err := readInstallation(f)
	if errors.Is(err, ErrNotFound) {
		return "ledger.json", nil
	}
	if err != nil {
		return "", err
	}
	if i.LedgerFile == "" {
		return "ledger.json", nil
	}
	if !addressPattern.MatchString(i.EarlierAddress) || i.Address != i.EarlierAddress || i.LedgerFile != "ledger-"+i.EarlierAddress+".json" {
		return "", ErrStorage
	}
	return i.LedgerFile, nil
}

// PolicyChange contains public policy data only. The audit is profile-wide so a
// wallet switch cannot hide the preceding wallet's pending announcements.
type PolicyChange struct {
	ID              string   `json:"id"`
	Time            string   `json:"time"`
	Source          string   `json:"source"`
	Class           string   `json:"class"`
	PreviousAddress string   `json:"previous_address"`
	Address         string   `json:"address"`
	Before          Settings `json:"before"`
	After           Settings `json:"after"`
	Applied         bool     `json:"applied"`
	Announced       bool     `json:"announced"`
}

type policyAudit struct {
	Version int            `json:"version"`
	Changes []PolicyChange `json:"changes"`
}

func policyDescription(c PolicyChange) string {
	amount := func(p *int64) string {
		if p == nil {
			return "none"
		}
		return "$" + Dollars(*p)
	}
	parts := []string{}
	if c.PreviousAddress != c.Address {
		parts = append(parts, "wallet: "+c.PreviousAddress+" -> "+c.Address)
	}
	for _, field := range limitFields(c.Before, c.After) {
		if !reflect.DeepEqual(field.old, field.new) {
			parts = append(parts, field.name+" limit: "+amount(field.old)+" -> "+amount(field.new))
		}
	}
	switches := append(switchFields(c.Before, c.After), switchField{"frozen", c.Before.Frozen, c.After.Frozen})
	for _, field := range switches {
		if field.old != field.new {
			parts = append(parts, fmt.Sprintf("%s: %t -> %t", field.name, field.old, field.new))
		}
	}
	return fmt.Sprintf("Policy %s at %s (%s): %s.", c.Class, c.Time, c.Source, strings.Join(parts, "; "))
}

func newPolicyChange(before, after Settings, previous, address, source string) PolicyChange {
	class := "tighten"
	if len(DescribeChanges(before, after)) != 0 || previous != address {
		class = "loosen"
	}
	return PolicyChange{ID: hex.EncodeToString(randomPolicyID()), Time: time.Now().UTC().Format(time.RFC3339Nano), Source: source, Class: class, PreviousAddress: previous, Address: address, Before: before, After: after}
}

func randomPolicyID() []byte {
	// crypto/rand.Read terminates the process if system randomness fails.
	b := make([]byte, 16)
	rand.Read(b)
	return b
}

func readPolicyAudit(files *Files) (policyAudit, error) {
	a := policyAudit{Version: 1, Changes: []PolicyChange{}}
	raw, err := files.Read("policy-audit.json")
	if errors.Is(err, ErrNotFound) {
		return a, nil
	}
	if err != nil {
		return a, err
	}
	if DecodeStrict(raw, &a, 32<<20) != nil || a.Version != 1 {
		return a, ErrStorage
	}
	for _, c := range a.Changes {
		if !hexID.MatchString(c.ID) || !addressPattern.MatchString(c.Address) || !addressPattern.MatchString(c.PreviousAddress) || c.Before.Validate() != nil || c.After.Validate() != nil {
			return a, ErrStorage
		}
	}
	return a, nil
}
func (a policyAudit) save(files *Files) error {
	raw, err := json.Marshal(a)
	if err != nil {
		return ErrStorage
	}
	return files.Write("policy-audit.json", raw)
}

// The intent is durable before the mutation. The revision stored atomically with
// the policy/selection recovers a crash between committing and marking the audit.
func (g *Engine) policyAuditLocked(v Vault) (policyAudit, error) {
	a, err := readPolicyAudit(g.Files)
	if err != nil {
		return a, err
	}
	i, err := readInstallation(g.Files)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return a, err
	}
	for j := range a.Changes {
		c := &a.Changes[j]
		if c.ID == v.PolicyRevision || c.ID == i.PolicyRevision {
			c.Applied = true
		}
	}
	return a, nil
}
func (g *Engine) preparePolicyLocked(v Vault, change PolicyChange) (policyAudit, error) {
	a, err := g.policyAuditLocked(v)
	if err != nil {
		return a, err
	}
	a.Changes = append(a.Changes, change)
	return a, a.save(g.Files)
}
func (g *Engine) savePolicyLocked(v Vault, next Settings, source string) error {
	if reflect.DeepEqual(v.Settings, next) {
		return nil
	}
	change := newPolicyChange(v.Settings, next, v.Address, v.Address, source)
	a, err := g.preparePolicyLocked(v, change)
	if err != nil {
		return err
	}
	v.Settings, v.PolicyRevision = next, change.ID
	if err = SaveVault(g.Store, v); err != nil {
		return err
	}
	a.Changes[len(a.Changes)-1].Applied = true
	return a.save(g.Files)
}
func (g *Engine) takePolicyNotices() ([]string, error) {
	if g == nil {
		return nil, nil
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
	a, err := g.policyAuditLocked(v)
	if err != nil {
		return nil, err
	}
	notices := []string{}
	for j := range a.Changes {
		c := &a.Changes[j]
		if c.Applied && !c.Announced {
			notices = append(notices, policyDescription(*c))
			c.Announced = true
		}
	}
	if len(notices) != 0 {
		err = a.save(g.Files)
	}
	return notices, err
}
