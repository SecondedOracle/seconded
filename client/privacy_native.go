package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// Public reviewed snapshots and invariant wording travel with the npx binary.
// No source checkout, Python interpreter, remote import or runtime download.
//
//go:embed privacy_data/*.json
var privacyData embed.FS

type privacyMap = map[string]any

func privacyObject(v any) privacyMap { m, _ := v.(map[string]any); return m }
func privacyString(v any) string     { s, _ := v.(string); return s }
func privacyArray(v any) []any       { a, _ := v.([]any); return a }
func privacyInt(v any) int64 {
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return i
	case int:
		return int64(n)
	case int64:
		return n
	}
	return 0
}
func privacyJSON(raw []byte) privacyMap {
	var out privacyMap
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&out) != nil {
		return nil
	}
	return out
}
func privacyClone(v any) any {
	b, e := json.Marshal(v)
	if e != nil {
		return nil
	}
	var out any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if d.Decode(&out) != nil {
		return nil
	}
	return out
}

var privacyDocuments sync.Map // name -> once-loaded immutable document

func privacyDocument(name string) privacyMap {
	if directory := os.Getenv("SECONDED_SNAPSHOT_DIR"); directory != "" && oneOf(name, "sanctions", "shielded-routes", "stealth-base") {
		path := filepath.Join(directory, name+".json")
		info, err := os.Lstat(path)
		if err == nil {
			if !info.Mode().IsRegular() || info.Size() > 40000000 {
				return nil
			}
			file, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer file.Close()
			raw, err := io.ReadAll(io.LimitReader(file, 40000001))
			if err != nil || len(raw) > 40000000 {
				return nil
			}
			doc := privacyJSON(raw)
			if name == "stealth-base" {
				// Only evidence time can change without a new identity review.
				baseline := decodePrivacyDocument(name)
				candidate := privacyObject(privacyClone(doc))
				delete(baseline, "verified_at")
				delete(candidate, "verified_at")
				if !reflect.DeepEqual(baseline, candidate) {
					return nil
				}
			}
			return doc // Product loaders still verify hashes, identities and age.
		}
		if !os.IsNotExist(err) {
			return nil
		}
	}
	loader, _ := privacyDocuments.LoadOrStore(name, sync.OnceValue(func() privacyMap { return decodePrivacyDocument(name) }))
	return privacyObject(privacyClone(loader.(func() privacyMap)()))
}
func decodePrivacyDocument(name string) privacyMap {
	b, e := privacyData.ReadFile("privacy_data/" + name + ".json")
	if e != nil {
		return nil
	}
	return privacyJSON(b)
}
func privacyText(name string) privacyMap { return privacyObject(privacyDocument("text")[name]) }
func privacyRejected() privacyMap {
	return privacyMap{"status": "rejected", "reason": "invalid_privacy_request"}
}

var privacyPatterns sync.Map // bounded by trusted schema and code patterns

func privacyMatch(pattern string, v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	loader, _ := privacyPatterns.LoadOrStore(pattern, sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile("^(?:" + pattern + ")$") }))
	return loader.(func() *regexp.Regexp)().MatchString(s)
}
func privacyAddress(v any, nonzero bool) bool {
	return privacyMatch(`0x[0-9a-fA-F]{40}`, v) && (!nonzero || strings.ToLower(privacyString(v)) != "0x"+strings.Repeat("0", 40))
}
func privacyAsset(v any) bool { return v == "native" || privacyAddress(v, true) }
func privacyAmount(v any, bits int, zero bool) bool {
	pattern := `[1-9][0-9]{0,77}`
	if zero {
		pattern = `0|[1-9][0-9]{0,36}`
	}
	if !privacyMatch(pattern, v) {
		return false
	}
	n, ok := new(big.Int).SetString(privacyString(v), 10)
	return ok && n.BitLen() <= bits
}
func privacySorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Python's sort_keys/ensure_ascii hash format is distinct from RFC 8785. Keep
// it for plan compatibility; signatures use canonicalValue (RFC 8785) instead.
func privacyCanonical(v any) ([]byte, error) {
	var b bytes.Buffer
	nodes := 0
	var write func(any, int) error
	write = func(v any, depth int) error {
		nodes++
		if depth > 16 || nodes > 1000000 {
			return ErrInvalid
		}
		switch x := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			b.WriteByte('{')
			for i, k := range keys {
				if i > 0 {
					b.WriteByte(',')
				}
				if err := write(k, depth+1); err != nil {
					return err
				}
				b.WriteByte(':')
				if err := write(x[k], depth+1); err != nil {
					return err
				}
			}
			b.WriteByte('}')
		case []any:
			b.WriteByte('[')
			for i, item := range x {
				if i > 0 {
					b.WriteByte(',')
				}
				if err := write(item, depth+1); err != nil {
					return err
				}
			}
			b.WriteByte(']')
		case string:
			if !utf8.ValidString(x) {
				return ErrInvalid
			}
			b.WriteByte('"')
			for _, r := range x {
				switch r {
				case '"', '\\':
					b.WriteByte('\\')
					b.WriteRune(r)
				case '\b':
					b.WriteString(`\b`)
				case '\f':
					b.WriteString(`\f`)
				case '\n':
					b.WriteString(`\n`)
				case '\r':
					b.WriteString(`\r`)
				case '\t':
					b.WriteString(`\t`)
				default:
					if r < 32 || r > 126 {
						if r > 0xffff {
							hi, lo := utf16.EncodeRune(r)
							fmt.Fprintf(&b, `\u%04x\u%04x`, hi, lo)
						} else {
							fmt.Fprintf(&b, `\u%04x`, r)
						}
					} else {
						b.WriteRune(r)
					}
				}
			}
			b.WriteByte('"')
		case nil:
			b.WriteString("null")
		case bool:
			if x {
				b.WriteString("true")
			} else {
				b.WriteString("false")
			}
		case json.Number:
			if strings.ContainsAny(string(x), ".eE") {
				// Only trusted wall-clock output uses fractional numbers;
				// input schemas still require strict JSON integers.
				value, err := x.Float64()
				if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
					return ErrInvalid
				}
				b.WriteString(string(x))
				return nil
			}
			if _, ok := new(big.Int).SetString(string(x), 10); !ok {
				return ErrInvalid
			}
			b.WriteString(string(x))
		case int:
			fmt.Fprint(&b, x)
		case int64:
			fmt.Fprint(&b, x)
		default:
			// Internal typed slices and structs only; input has already been decoded.
			next := privacyClone(v)
			if next == nil {
				return ErrInvalid
			}
			return write(next, depth+1)
		}
		return nil
	}
	if err := write(v, 0); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func privacyHash(v any) string {
	raw, e := privacyCanonical(v)
	if e != nil {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func privacySchema(schema any, value any, depth int) bool {
	s := privacyObject(schema)
	if s == nil || depth > 12 {
		return false
	}
	if alternatives, ok := s["oneOf"]; ok {
		count := 0
		for _, a := range privacyArray(alternatives) {
			if privacySchema(a, value, depth+1) {
				count++
			}
		}
		return count == 1
	}
	if c, ok := s["const"]; ok && !reflect.DeepEqual(c, value) {
		return false
	}
	if e, ok := s["enum"]; ok && !portfolioContains(e, value) {
		return false
	}
	switch s["type"] {
	case "object":
		m := privacyObject(value)
		if m == nil {
			return false
		}
		props := privacyObject(s["properties"])
		for _, k := range privacyArray(s["required"]) {
			if _, ok := m[privacyString(k)]; !ok {
				return false
			}
		}
		for k, v := range m {
			p, ok := props[k]
			if !ok || !privacySchema(p, v, depth+1) {
				return false
			}
		}
	case "array":
		a, ok := value.([]any)
		if !ok || int64(len(a)) < privacyInt(s["minItems"]) || int64(len(a)) > privacyInt(s["maxItems"]) {
			return false
		}
		seen := map[string]bool{}
		for _, v := range a {
			if !privacySchema(s["items"], v, depth+1) {
				return false
			}
			if s["uniqueItems"] == true {
				h := privacyHash(v)
				if seen[h] {
					return false
				}
				seen[h] = true
			}
		}
	case "integer":
		n, ok := value.(json.Number)
		if !ok {
			return false
		}
		i, e := n.Int64()
		if e != nil || i < privacyInt(s["minimum"]) || i > privacyInt(s["maximum"]) {
			return false
		}
	case "string":
		v, ok := value.(string)
		if !ok {
			return false
		}
		max := int64(256)
		if n, ok := s["maxLength"]; ok {
			max = privacyInt(n)
		}
		if int64(utf8.RuneCountInString(v)) > max {
			return false
		}
		if p, ok := s["pattern"]; ok && !privacyMatch(privacyString(p), v) {
			return false
		}
	}
	return true
}

type privacySavedPlan struct {
	raw             []byte
	request         []byte
	issued, expires time.Time
}

// Native state is private to one MCP session, bounded and never serialized.
// Production callers cannot inject clocks, randomness, snapshots or trust data.
type privacyNative struct {
	now                     func() time.Time
	random                  io.Reader
	spend, view             *secp.PrivateKey
	receiveCalls            int
	used, invoices          map[string]bool
	receivePlans, swapPlans map[string]privacySavedPlan
	closed                  bool
}

func newPrivacyNative() *privacyNative {
	return &privacyNative{now: time.Now, random: rand.Reader, used: map[string]bool{}, invoices: map[string]bool{}, receivePlans: map[string]privacySavedPlan{}, swapPlans: map[string]privacySavedPlan{}}
}
func (n *privacyNative) close() {
	if n.spend != nil {
		n.spend.Zero()
	}
	if n.view != nil {
		n.view.Zero()
	}
	n.spend = nil
	n.view = nil
	clear(n.receivePlans)
	clear(n.swapPlans)
	clear(n.used)
	clear(n.invoices)
	n.closed = true
}
func (n *privacyNative) call(name string, input privacyMap) privacyMap {
	if n.closed {
		return privacyRejected()
	}
	input = privacyObject(privacyNormalizeIntegers(input))
	guide, ok := privacyTool("seconded_" + name)
	if !ok || !privacySchema(guide.InputSchema, input, 0) {
		return privacyRejected()
	}
	var out privacyMap
	switch name {
	case "privacy_shielded_route":
		out = n.shieldedRoute(input, nil, nil)
	case "privacy_route_check":
		out = n.route(input)
	case "privacy_pool_check":
		out = privacyLocalPool(input)
	case "private_receive_prepare":
		out = n.receive(input)
	case "private_purchase_prepare":
		out = n.purchase(input, nil, nil)
	case "private_swap_quote":
		out = n.swap(input)
	case "private_fact_prove":
		out = n.fact(input, nil, nil)
	}
	if out == nil {
		return privacyRejected()
	}
	return out
}
func (p *privacyProcess) call(ctx context.Context, raw []byte) any {
	// The old companion is an explicit developer-only opt-in, never a fallback.
	if privacyCompanionEnabled() {
		return p.companionCall(ctx, raw)
	}
	if !p.lock(ctx) {
		return privacyMap{"status": "unknown", "reason": "local_privacy_runtime_unavailable", "charged": "no"}
	}
	defer func() { <-p.gate }()
	if p.closed {
		return privacyRejected()
	}
	var request privacyMap
	if DecodeStrict(raw, &request, 32*1024) != nil || len(request) != 2 || privacyObject(request["input"]) == nil {
		return privacyMap{"status": "rejected", "reason": "invalid_privacy_request", "charged": "no"}
	}
	if p.native == nil {
		p.native = newPrivacyNative()
	}
	out := p.native.call(privacyString(request["tool"]), privacyObject(request["input"]))
	out["charged"] = "no"
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded) > 256*1024 {
		return privacyMap{"status": "rejected", "reason": "invalid_privacy_request", "charged": "no"}
	}
	return out
}

func privacyTrustedDocument(name string) privacyMap {
	root := os.Getenv("SECONDED_PRIVACY_TRUST")
	if !filepath.IsAbs(root) || filepath.Base(name) != name {
		return nil
	}
	// The owner selects this directory, never the model. Open without following
	// links and validate the descriptor before reading (including FIFO refusal).
	f, e := openPrivacyTrustDocument(filepath.Join(root, name))
	if e != nil {
		return nil
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, 32769))
	if e != nil {
		return nil
	}
	var out privacyMap
	if DecodeStrict(raw, &out, 32768) != nil {
		return nil
	}
	return out
}

// Python parses JSON -0 as integer zero. Normalize before binding hashes so the
// Go request has the same semantics and canonical bytes as the reference.
func privacyNormalizeIntegers(value any) any {
	switch v := value.(type) {
	case json.Number:
		if v == "-0" {
			return json.Number("0")
		}
	case map[string]any:
		result := make(map[string]any, len(v))
		for k, item := range v {
			result[k] = privacyNormalizeIntegers(item)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = privacyNormalizeIntegers(item)
		}
		return result
	}
	return value
}

func privacyWallSeconds(now time.Time) float64 {
	return float64(now.Unix()) + float64(now.Nanosecond())/1e9
}
func privacyWallNumber(now time.Time) any {
	if now.Nanosecond() == 0 {
		return now.Unix()
	}
	value := strconv.FormatFloat(privacyWallSeconds(now), 'f', -1, 64)
	if !strings.Contains(value, ".") {
		value += ".0"
	}
	return json.Number(value)
}
