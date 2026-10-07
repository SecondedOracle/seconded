package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
)

const ResponseLimit = 64 << 10
const MessageLimit = 1 << 20

var ErrInvalid = errors.New("invalid_response")

// Check JSON before decoding: encoding/json alone accepts duplicate keys and
// repairs invalid Unicode, both of which can change the signed interpretation.
func checkJSON(data []byte, limit int) error {
	return checkJSONIntegers(data, limit, "")
}

// wideIntegerPath is reserved for unsigned local ledger retry timestamps.
// Signed/wire JSON continues to require exactly representable JCS integers.
func checkJSONIntegers(data []byte, limit int, wideIntegerPath string) error {
	if len(data) > limit || !utf8.Valid(data) {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	nodes := 0
	var walk func(int, string) error
	walk = func(depth int, path string) error {
		nodes++
		if depth > 64 || nodes > 100000 {
			return ErrInvalid
		}
		t, err := d.Token()
		if err != nil {
			return ErrInvalid
		}
		if n, ok := t.(json.Number); ok && !strings.ContainsAny(string(n), ".eE") {
			v, ok := new(big.Int).SetString(string(n), 10)
			wide := wideIntegerPath != "" && path == wideIntegerPath && ok && v.IsInt64() && v.Sign() >= 0
			if !ok || (!wide && new(big.Int).Abs(v).Cmp(big.NewInt(9007199254740991)) > 0) {
				return ErrInvalid
			}
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					k, e := d.Token()
					if e != nil {
						return ErrInvalid
					}
					key, ok := k.(string)
					if !ok || seen[key] || len(seen) >= 10000 {
						return ErrInvalid
					}
					seen[key] = true
					escaped := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
					if e = walk(depth+1, path+"/"+escaped); e != nil {
						return e
					}
				}
				end, e := d.Token()
				if e != nil || end != json.Delim('}') {
					return ErrInvalid
				}
			case '[':
				for d.More() {
					if e := walk(depth+1, path+"/*"); e != nil {
						return e
					}
				}
				end, e := d.Token()
				if e != nil || end != json.Delim(']') {
					return ErrInvalid
				}
			default:
				return ErrInvalid
			}
		}
		return nil
	}
	if err := walk(0, ""); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	// JCS also rejects unpaired UTF-16 surrogate escapes and nonfinite numbers.
	if _, err := canonicalTransform(data); err != nil {
		return ErrInvalid
	}
	return nil
}

func DecodeStrict(data []byte, dst any, limit int) error {
	if err := checkJSON(data, limit); err != nil {
		return err
	}
	return decodeStrictChecked(data, dst)
}

func decodeStrictChecked(data []byte, dst any) error {
	if err := exactShape(data, reflect.TypeOf(dst)); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	d.UseNumber()
	if err := d.Decode(dst); err != nil {
		return ErrInvalid
	}
	return nil
}

// encoding/json otherwise matches struct fields case-insensitively and silently
// turns null into zero values. Payment schemas require exact names and presence.
func exactShape(data []byte, t reflect.Type) error {
	if t == nil {
		return ErrInvalid
	}
	// PortfolioReport validates every field against its generated closed schema
	// in UnmarshalJSON; its Go representation deliberately retains the JSON tree.
	if t == reflect.TypeOf(PortfolioReport{}) {
		return nil
	}
	if t == reflect.TypeOf(json.RawMessage{}) {
		return nil
	}
	if t.Kind() == reflect.Pointer {
		if bytes.Equal(data, []byte("null")) {
			return nil
		}
		return exactShape(data, t.Elem())
	}
	if bytes.Equal(data, []byte("null")) {
		if t.Kind() == reflect.Interface {
			return nil
		}
		return ErrInvalid
	}
	switch t.Kind() {
	case reflect.Struct:
		var obj map[string]json.RawMessage
		if json.Unmarshal(data, &obj) != nil {
			return ErrInvalid
		}
		fields := map[string]reflect.StructField{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, opts, _ := strings.Cut(tag, ",")
			if name == "" {
				name = f.Name
			}
			fields[name] = f
			if !strings.Contains(opts, "omitempty") && !strings.Contains(opts, "omitzero") {
				if _, ok := obj[name]; !ok {
					return ErrInvalid
				}
			}
		}
		for name, b := range obj {
			f, ok := fields[name]
			if !ok {
				return ErrInvalid
			}
			// Fields tagged seconded:"nullable" explicitly support null on the wire or in
			// legacy storage; null reads as the zero value. Other value fields still reject null.
			if f.Tag.Get("seconded") == "nullable" && bytes.Equal(b, []byte("null")) {
				continue
			}
			if err := exactShape(b, f.Type); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return nil
		}
		var arr []json.RawMessage
		if json.Unmarshal(data, &arr) != nil {
			return ErrInvalid
		}
		for _, b := range arr {
			if err := exactShape(b, t.Elem()); err != nil {
				return err
			}
		}
	case reflect.Map:
		var obj map[string]json.RawMessage
		if json.Unmarshal(data, &obj) != nil {
			return ErrInvalid
		}
		for _, b := range obj {
			if err := exactShape(b, t.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

func Canonical(data []byte) ([]byte, error) {
	if err := checkJSON(data, MessageLimit); err != nil {
		return nil, err
	}
	return canonicalTransform(data)
}

func canonicalTransform(data []byte) ([]byte, error) {
	wrapped := make([]byte, 0, len(data)+2)
	wrapped = append(wrapped, '[')
	wrapped = append(wrapped, data...)
	wrapped = append(wrapped, ']')
	b, e := jsoncanonicalizer.Transform(wrapped)
	if e != nil || len(b) < 2 {
		return nil, ErrInvalid
	}
	return b[1 : len(b)-1], nil
}

func canonicalValue(v any) ([]byte, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return nil, ErrInvalid
	}
	return Canonical(b)
}
