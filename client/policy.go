package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

const (
	ChainID                = 84532
	Network                = "eip155:84532"
	Asset                  = "0x036cbd53842c5426634e7929541ec2318f3dcf7e"
	PayTo                  = "0x010ab46d566cde25cca0ee55eb105e781c7bcf3a"
	TokenName              = "USDC"
	TokenVersion           = "2"
	MaxAuthorization int64 = 2500000
	ChatDailyCeiling int64 = 25000000
	ValiditySeconds  int64 = 300
)

// DefaultPaymentNetwork is where a new check pays when it names no network. Network stays Base
// Sepolia: explicit base_sepolia, network-less legacy ledger entries and the parity vectors use it.
// The build-tag-specific deployment file selects the new-payment default.

var hexID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var hexDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)
var hexWord = regexp.MustCompile(`^0x[0-9a-f]{64}$`)
var addressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
var decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,6})?$`)

func atomic(s string) (int64, error) {
	if !regexp.MustCompile(`^(0|[1-9][0-9]{0,15})$`).MatchString(s) {
		return 0, ErrInvalid
	}
	v, e := strconv.ParseInt(s, 10, 64)
	if e != nil {
		return 0, ErrInvalid
	}
	return v, nil
}
func USD(s string) (int64, error) {
	if !decimalPattern.MatchString(s) {
		return 0, ErrInvalid
	}
	p := strings.Split(s, ".")
	if len(p[0]) > 9 {
		return 0, ErrInvalid
	}
	frac := ""
	if len(p) == 2 {
		frac = p[1]
	}
	for len(frac) < 6 {
		frac += "0"
	}
	v, e := strconv.ParseInt(p[0]+frac, 10, 64)
	if e != nil {
		return 0, ErrInvalid
	}
	return v, nil
}
func Dollars(v int64) string { return strconv.FormatInt(v/1000000, 10) + "." + fmtFraction(v%1000000) }
func fmtFraction(v int64) string {
	s := strconv.FormatInt(v+1000000, 10)[1:]
	for len(s) > 2 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	return s
}

// tiers carries generic byte boundaries; product prices come from the generated server catalogue.
var tiers = []struct {
	name     string
	maxBytes int
	amount   int64
}{
	{"small", 8192, productPrices["scam_check"]["small"]},
	{"medium", 65536, productPrices["scam_check"]["medium"]},
	{"large", 131072, productPrices["scam_check"]["large"]},
}

// productTiers is the launch product set and how many tiers each sells. Trade has no large tier, and Stock Token
// Token and Agent Registry are small only: larger inputs are refused before signing at an unsupported tier.
var productTiers = offeredProductTiers(productGuideJSON)
var productPrices = cataloguePrices(productGuideJSON)
var networkProductPrices = catalogueNetworkPrices(productGuideJSON)

func catalogueNetworkPrices(data []byte) map[string]map[string]map[string]int64 {
	var guide struct {
		Products []struct {
			ID     string                       `json:"id"`
			Prices map[string]map[string]string `json:"price_usd_by_network"`
		} `json:"products"`
	}
	if err := json.Unmarshal(data, &guide); err != nil {
		panic(err)
	}
	result := map[string]map[string]map[string]int64{}
	for _, product := range guide.Products {
		result[product.ID] = map[string]map[string]int64{}
		for network, tiers := range product.Prices {
			result[product.ID][network] = map[string]int64{}
			for tier, dollars := range tiers {
				amount, err := USD(dollars)
				if err != nil || amount <= 0 || amount > MaxAuthorization {
					panic("invalid network catalogue price")
				}
				result[product.ID][network][tier] = amount
			}
		}
	}
	return result
}

// ProductPriceOnNetwork is the exact expected price for the payment network.
func ProductPriceOnNetwork(product string, size int, network string) (string, int64, error) {
	tier, amount, err := ProductPrice(product, size)
	if err != nil {
		return tier, amount, err
	}
	if override, ok := networkProductPrices[product][network]; ok {
		var found bool
		amount, found = override[tier]
		if !found {
			return "", 0, ErrInvalid
		}
	}
	return tier, amount, nil
}

func cataloguePrices(data []byte) map[string]map[string]int64 {
	var guide struct {
		Products []struct {
			ID     string            `json:"id"`
			Prices map[string]string `json:"price_usd"`
		} `json:"products"`
	}
	if err := json.Unmarshal(data, &guide); err != nil {
		panic(err)
	}
	prices := map[string]map[string]int64{}
	for _, product := range guide.Products {
		prices[product.ID] = map[string]int64{}
		for tier, dollars := range product.Prices {
			amount, err := USD(dollars)
			if err != nil || amount <= 0 || amount > MaxAuthorization {
				panic("invalid catalogue price")
			}
			prices[product.ID][tier] = amount
		}
	}
	return prices
}

func offeredProductTiers(data []byte) map[string]int {
	var guide struct {
		Products []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"products"`
	}
	if err := json.Unmarshal(data, &guide); err != nil {
		panic(err)
	}
	known := map[string]int{
		"portfolio_check": 1, "cross_chain_compare": 1, "trade_check": 2, "stock_token_check": 1, "token_check": 1,
		"agent_registry_check": 1, "counterparty_check": 1, "lending_check": 1, "hidden_prompt_check": 3, "scam_check": 3,
	}
	offered := map[string]int{}
	known = deploymentProductTiers(known)
	for _, p := range guide.Products {
		if sold, ok := known[p.ID]; ok && p.Status == "available" {
			offered[p.ID] = sold
		}
	}
	return offered
}

// Retired before launch and never quoted or signed again. Ledgers written by earlier builds still name them,
// so they stay known to ledger validation.
var retiredProducts = map[string]bool{"transaction_check": true, "full_stock_check": true, "owner_instruction_check": true}

func tierPrice(size, sold int) (string, int64, error) {
	if size <= 0 {
		return "", 0, ErrInvalid
	}
	for _, t := range tiers[:sold] {
		if size <= t.maxBytes {
			return t.name, t.amount, nil
		}
	}
	return "", 0, errors.New("input_too_large")
}

// Price is the full tier table, before any product's tier limit.
func Price(size int) (string, int64, error) { return tierPrice(size, len(tiers)) }

// ProductPrice binds a product's tier limit to quotes, authorizations and commitments.
func ProductPrice(product string, size int) (string, int64, error) {
	sold, offered := productTiers[product]
	if !offered {
		return "", 0, ErrInvalid
	}
	tier, amount, err := tierPrice(size, sold)
	if err != nil {
		return tier, amount, err
	}
	if prices, ok := productPrices[product]; ok {
		amount, ok = prices[tier]
		if !ok {
			return "", 0, ErrInvalid
		}
	}
	return tier, amount, nil
}
func productOffered(s string) bool {
	_, ok := productTiers[s]
	return ok
}
func productOK(s string) bool {
	return s == portfolioProduct || s == comparisonProduct || productOffered(s) || retiredProducts[s] || s == "hidden_prompt_check" || s == "counterparty_check" || s == "lending_check"
}

type Request struct {
	Product string          `json:"product"`
	Input   json.RawMessage `json:"input"`
	Options Options         `json:"options"`
}
type Options struct {
	Network  string `json:"network"`
	Payer    string `json:"payer,omitempty"`
	MaxPrice string `json:"max_price,omitempty"`
}

func (r Request) Size() (int, error) {
	if err := validateAddressCharacters(r.Input); err != nil {
		return 0, err
	}
	if !deploymentPaymentNetwork(r.Options.Network) {
		return 0, ErrInvalid
	}
	if !productOffered(r.Product) || r.Options.Network == "" || !supportedNetwork(r.Options.Network) {
		return 0, ErrInvalid
	}
	if r.Product == portfolioProduct {
		if _, err := portfolioRequestScope(r); err != nil {
			return 0, err
		}
	}
	if r.Product == comparisonProduct {
		if _, err := comparisonRequestHash(r); err != nil {
			return 0, err
		}
	}
	if r.Product == "lending_check" {
		var input struct {
			Network  string `json:"network"`
			Account  string `json:"account"`
			MarketID string `json:"market_id"`
		}
		if DecodeStrict(r.Input, &input, MessageLimit) != nil ||
			!oneOf(input.Network, "eip155:8453", "eip155:5042") ||
			!addressPattern.MatchString(input.Account) ||
			!regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`).MatchString(input.MarketID) {
			return 0, ErrInvalid
		}
	}
	if r.Product == "counterparty_check" {
		var input struct {
			Network string `json:"network"`
			Address string `json:"address"`
		}
		if DecodeStrict(r.Input, &input, MessageLimit) != nil || !oneOf(input.Network, "eip155:8453", "eip155:5042", "eip155:4663") || !addressPattern.MatchString(input.Address) {
			return 0, ErrInvalid
		}
	}
	b, e := Canonical(r.Input)
	if e != nil || len(b) == 0 || b[0] != '{' {
		return 0, ErrInvalid
	}
	return len(b), nil
}
func (r Request) Digest() string {
	b, _ := Canonical(r.Input)
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}
func (r Request) Commitment(salt string) (string, error) {
	if !hexDigest.MatchString(salt) {
		return "", ErrInvalid
	}
	s, _ := hex.DecodeString(salt)
	b, e := r.canonicalRequest()
	if e != nil {
		return "", e
	}
	h := sha256.New()
	h.Write([]byte("seconded-request/v1"))
	h.Write(s)
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// v3.1 §6.3 binds the financial and product contract, not transport options. The keys and literals are
// server/payments/quotes.py canonical_body(); tests/contract fixtures carry the server's commitments.
func (r Request) canonicalRequest() ([]byte, error) {
	size, err := r.Size()
	if err != nil {
		return nil, err
	}
	tier, amount, err := ProductPriceOnNetwork(r.Product, size, r.Options.Network)
	if err != nil {
		return nil, err
	}
	return canonicalValue(map[string]any{
		"v": 1, "input": r.Input, "product": r.Product,
		"product_schema_version": "1", "predicate_version": "1",
		"tier": tier, "price_atomic": strconv.FormatInt(amount, 10),
		"asset": r.Options.Network + "/erc20:" + pins(r.Options.Network).Asset, "network": r.Options.Network,
		"scheme": "exact", "pay_to": PayTo, "billing_mode": "paid",
	})
}

// Nil is an explicit absence of a user budget; hard payment policy still applies.
type Limits struct {
	PerCheck    *int64 `json:"per_check"`
	Hour        *int64 `json:"hour"`
	Day         *int64 `json:"day"`
	Outstanding *int64 `json:"outstanding"`
}
type Settings struct {
	Limits    Limits `json:"limits"`
	ValueGate bool   `json:"value_gate"`
	Dedupe    bool   `json:"dedupe"`
	LoopBrake bool   `json:"loop_brake"`
	Alerts    bool   `json:"alerts"`
	Frozen    bool   `json:"frozen"`
}

func DefaultSettings() Settings {
	perCheck := MaxAuthorization
	return Settings{Limits: Limits{PerCheck: &perCheck}, ValueGate: false, Dedupe: true, LoopBrake: true, Alerts: true}
}
func (s Settings) Validate() error {
	for _, p := range []*int64{s.Limits.PerCheck, s.Limits.Hour, s.Limits.Day, s.Limits.Outstanding} {
		if p != nil && (*p < 0 || *p > 999999999999999) {
			return ErrInvalid
		}
	}
	return nil
}

// ChatPolicy permits tightening within the absolute $25 daily chat ceiling.
// Higher daily limits and every loosening require owner-terminal approval.
// Unchanged owner-approved daily limits permit unrelated safety changes.
func ChatPolicy(current, next Settings) error {
	if err := next.Validate(); err != nil {
		return err
	}
	if next.Limits.Day != nil && *next.Limits.Day > ChatDailyCeiling &&
		(current.Limits.Day == nil || *next.Limits.Day != *current.Limits.Day) {
		return ErrTerminalPolicyRequired
	}
	if current.Frozen && !next.Frozen || current.Dedupe && !next.Dedupe || current.LoopBrake && !next.LoopBrake {
		return ErrTerminalPolicyRequired
	}
	if len(DescribeChanges(current, next)) != 0 {
		return ErrTerminalPolicyRequired
	}
	return nil
}
