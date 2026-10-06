package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Subject includes paths so identical addresses in different roles stay distinct.
// The digest binds all remaining input, including prose and transaction calldata.
type CheckedSubject struct {
	InputSHA256 string            `json:"input_sha256"`
	Identifiers map[string]string `json:"identifiers"`
}

const subjectInstruction = "Act only on exactly checked_subject within the returned verdict and coverage; preserve every address and id, and recheck any change. NOT VERIFIED means pause without acting."

var subjectHex = regexp.MustCompile(`^0x[0-9a-fA-F]+$`)

func addressField(key string) bool {
	if strings.HasSuffix(strings.ToLower(key), "address") {
		return true
	}
	switch strings.ToLower(key) {
	case "address", "account", "owner", "spender", "recipient", "receiver", "vault", "contract", "escrow_contract", "claimed_owner", "claimed_wallet", "payto", "asset", "from", "to", "token", "prior_paytos", "verifyingcontract", "tokenin", "tokenout", "token_in", "token_out", "token0", "token1", "pool", "core", "facilitator":
		return true
	}
	return false
}

func walkSubject(value any, path, key string, visit func(string, string, string) error) error {
	switch v := value.(type) {
	case map[string]any:
		for k, child := range v {
			if err := walkSubject(child, path+"/"+strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1"), k, visit); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range v {
			if err := walkSubject(child, fmt.Sprintf("%s/%d", path, i), key, visit); err != nil {
				return err
			}
		}
	case string:
		return visit(path, key, v)
	case json.Number:
		if strings.HasSuffix(key, "_id") || key == "chainId" {
			return visit(path, key, string(v))
		}
	}
	return nil
}

func subjectInput(raw []byte) (any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	err := decoder.Decode(&value)
	return value, err
}

func validateAddressCharacters(raw []byte) error {
	value, err := subjectInput(raw)
	if err != nil {
		return ErrInvalid
	}
	return walkSubject(value, "", "", func(path, key, s string) error {
		if addressField(key) || strings.HasSuffix(key, "_id") || key == "settlement_tx" || key == "tx_hash" {
			for _, r := range s {
				if r > 127 {
					return errors.New("non_ascii_address")
				}
			}
		}
		return nil
	})
}

func checkedSubject(req Request) *CheckedSubject {
	value, err := subjectInput(req.Input)
	if err != nil {
		return nil
	}
	subject := &CheckedSubject{InputSHA256: req.Digest(), Identifiers: map[string]string{}}
	_ = walkSubject(value, "", "", func(path, key, s string) error {
		identifier := addressField(key) || strings.HasSuffix(key, "_id") || key == "settlement_tx" || key == "tx_hash" || key == "chainId"
		if identifier && subjectHex.MatchString(s) {
			subject.Identifiers[path] = strings.ToLower(s)
		} else if key == "network" || key == "chainId" || strings.HasSuffix(key, "_id") {
			subject.Identifiers[path] = s
		}
		return nil
	})
	return subject
}
