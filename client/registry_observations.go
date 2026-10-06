package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

type RegistryClaim struct {
	Claim             *string `json:"claim"`
	Result            string  `json:"result"`
	Status            string  `json:"status"`
	Reason            *string `json:"reason,omitempty"`
	RegisteredAddress *string `json:"registered_address,omitempty"`
}
type RegistryClaims struct {
	Owner  RegistryClaim `json:"owner"`
	Wallet RegistryClaim `json:"wallet"`
}
type RegistryRelationship struct {
	Status             string  `json:"status"`
	Reason             *string `json:"reason,omitempty"`
	Address            *string `json:"address,omitempty"`
	Records            *int    `json:"records,omitempty"`
	DistinctSubmitters *int    `json:"distinct_submitters,omitempty"`
	ExtraRecords       *int    `json:"extra_records,omitempty"`
}
type RegistryRelationships struct {
	Owner             RegistryRelationship `json:"owner"`
	Wallet            RegistryRelationship `json:"wallet"`
	Repeated          RegistryRelationship `json:"repeated"`
	Revoked           RegistryRelationship `json:"revoked"`
	Basis             *string              `json:"basis,omitempty"`
	SampleCount       *int                 `json:"sample_count,omitempty"`
	BoundedReplyCount *int                 `json:"bounded_reply_count,omitempty"`
	OmittedRecords    *int                 `json:"omitted_records,omitempty"`
	OutsideReply      *string              `json:"outside_reply,omitempty"`
}
type RegistryObservations struct {
	Schema                string                `json:"schema"`
	Network               string                `json:"network"`
	AgentID               string                `json:"agent_id"`
	EvidenceSHA256        string                `json:"evidence_sha256"`
	AddressClaims         RegistryClaims        `json:"address_claims"`
	FeedbackRelationships RegistryRelationships `json:"feedback_relationships"`
}

var registryAddress = regexp.MustCompile(`^0x[0-9a-f]{40}$`)
var registryID = regexp.MustCompile(`^(0|[1-9][0-9]{0,77})$`)

const zeroRegistryAddress = "0x0000000000000000000000000000000000000000"

func validRegistryID(s string) bool {
	n, ok := new(big.Int).SetString(s, 10)
	return registryID.MatchString(s) && ok && n.Sign() >= 0 && n.BitLen() <= 256
}
func registryNonzero(s string) bool {
	return registryAddress.MatchString(s) && s != zeroRegistryAddress
}
func valueIs(s *string, want string) bool { return s != nil && *s == want }
func validRegistryClaim(c RegistryClaim, wallet bool) bool {
	if c.Claim == nil {
		return c.Status == "not_checked" && c.Result == "not_supplied" && valueIs(c.Reason, "claim_not_supplied") && c.RegisteredAddress == nil
	}
	if !registryNonzero(*c.Claim) {
		return false
	}
	if c.Status == "unavailable" {
		return c.Result == "unavailable" && c.RegisteredAddress == nil && c.Reason != nil &&
			(oneOf(*c.Reason, "identity_unavailable", "not_registered") || wallet && *c.Reason == "wallet_unavailable")
	}
	if c.Status != "checked" || c.Reason != nil || c.RegisteredAddress == nil || !registryAddress.MatchString(*c.RegisteredAddress) {
		return false
	}
	if *c.RegisteredAddress == zeroRegistryAddress {
		return wallet && c.Result == "wallet_unset"
	}
	if *c.Claim == *c.RegisteredAddress {
		return c.Result == "match"
	}
	return c.Result == "mismatch"
}
func boundedCount(n *int, max int) bool { return n != nil && *n >= 0 && *n <= max }
func validRegistryObservations(o *RegistryObservations) bool {
	if o == nil {
		return true
	}
	if o.Schema != "seconded-registry-observations/v1" || !oneOf(o.Network, "eip155:8453", "eip155:5042", "eip155:4663") || !validRegistryID(o.AgentID) || !configFingerprint.MatchString(o.EvidenceSHA256) || !validRegistryClaim(o.AddressClaims.Owner, false) || !validRegistryClaim(o.AddressClaims.Wallet, true) {
		return false
	}
	f := o.FeedbackRelationships
	metadata := f.Basis != nil
	if !metadata && (f.SampleCount != nil || f.BoundedReplyCount != nil || f.OmittedRecords != nil || f.OutsideReply != nil) {
		return false
	}
	if metadata && (!valueIs(f.Basis, "displayed_reputation_entries_including_revoked") || !valueIs(f.OutsideReply, "not_checked") || !boundedCount(f.SampleCount, 32) || !boundedCount(f.BoundedReplyCount, 256) || !boundedCount(f.OmittedRecords, 256) || *f.SampleCount+*f.OmittedRecords != *f.BoundedReplyCount) {
		return false
	}
	for role, r := range map[string]RegistryRelationship{"owner": f.Owner, "wallet": f.Wallet, "repeated": f.Repeated, "revoked": f.Revoked} {
		measured := oneOf(r.Status, "checked", "partial")
		if !measured {
			if r.Reason == nil || r.Address != nil || r.Records != nil || r.DistinctSubmitters != nil || r.ExtraRecords != nil {
				return false
			}
			if !metadata {
				if r.Status != "unavailable" || *r.Reason != "feedback_unavailable" {
					return false
				}
			} else if *f.SampleCount == 0 && *f.OmittedRecords > 0 {
				if r.Status != "not_checked" || *r.Reason != "no_displayed_records" {
					return false
				}
			} else {
				if !oneOf(role, "owner", "wallet") {
					return false
				}
				switch *r.Reason {
				case "not_registered":
					if r.Status != "not_checked" {
						return false
					}
				case "wallet_unset":
					if role != "wallet" || r.Status != "not_checked" {
						return false
					}
				case "identity_unavailable":
					if r.Status != "unavailable" {
						return false
					}
				case "owner_unavailable", "owner_unset":
					if role != "owner" || r.Status != "unavailable" {
						return false
					}
				case "wallet_unavailable":
					if role != "wallet" || r.Status != "unavailable" {
						return false
					}
				default:
					return false
				}
			}
			continue
		}
		if !metadata || (*f.SampleCount == 0 && *f.OmittedRecords > 0) {
			return false
		}
		if *f.OmittedRecords > 0 {
			if r.Status != "partial" || !valueIs(r.Reason, "displayed_records_only") {
				return false
			}
		} else if r.Status != "checked" || r.Reason != nil {
			return false
		}
		if role == "repeated" {
			if r.Address != nil || r.Records != nil || !boundedCount(r.DistinctSubmitters, *f.SampleCount/2) || !boundedCount(r.ExtraRecords, *f.SampleCount) || *r.DistinctSubmitters > *r.ExtraRecords || (*r.DistinctSubmitters == 0) != (*r.ExtraRecords == 0) || (*f.SampleCount > 0 && *r.ExtraRecords >= *f.SampleCount) {
				return false
			}
		} else {
			if !boundedCount(r.Records, *f.SampleCount) || r.DistinctSubmitters != nil || r.ExtraRecords != nil {
				return false
			}
			if role == "revoked" {
				if r.Address != nil {
					return false
				}
			} else {
				if r.Address == nil || !registryNonzero(*r.Address) {
					return false
				}
				claim := o.AddressClaims.Owner
				if role == "wallet" {
					claim = o.AddressClaims.Wallet
				}
				if claim.RegisteredAddress != nil && *claim.RegisteredAddress != *r.Address {
					return false
				}
			}
		}
	}
	return true
}

// Optional keys are absent, never null. Compare the strict decoded shape with
// the original to preserve that distinction without permitting arbitrary maps.
func (o *RegistryObservations) UnmarshalJSON(data []byte) error {
	type wire RegistryObservations
	var v wire
	if DecodeStrict(data, &v, ResponseLimit) != nil {
		return ErrInvalid
	}
	candidate := RegistryObservations(v)
	if !validRegistryObservations(&candidate) {
		return ErrInvalid
	}
	original, err := Canonical(data)
	normalized, err2 := canonicalValue(v)
	if err != nil || err2 != nil || string(original) != string(normalized) {
		return ErrInvalid
	}
	*o = candidate
	return nil
}

func registryClaimsHash(network, id string, owner, wallet *string) string {
	raw, err := canonicalValue(map[string]any{"network": network, "agent_id": id, "claimed_owner": owner, "claimed_wallet": wallet})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Retain only a digest of the submitted identity/claims alongside the existing
// request commitment. Receipt-only recovery can then validate after restart.
func registryRequestHash(req Request) (string, error) {
	if req.Product != "agent_registry_check" {
		return "", nil
	}
	var input struct {
		Network string          `json:"network"`
		AgentID json.RawMessage `json:"agent_id"`
		Owner   *string         `json:"claimed_owner,omitempty"`
		Wallet  *string         `json:"claimed_wallet,omitempty"`
		Context *string         `json:"context,omitempty"`
	}
	if DecodeStrict(req.Input, &input, MessageLimit) != nil {
		return "", ErrInvalid
	}
	id := string(input.AgentID)
	if strings.HasPrefix(id, `"`) {
		if json.Unmarshal(input.AgentID, &id) != nil {
			return "", ErrInvalid
		}
	}
	if !validRegistryID(id) || !oneOf(input.Network, "eip155:8453", "eip155:5042", "eip155:4663") {
		return "", ErrInvalid
	}
	for _, c := range []*string{input.Owner, input.Wallet} {
		if c != nil {
			*c = strings.ToLower(*c)
			if !registryNonzero(*c) {
				return "", ErrInvalid
			}
		}
	}
	return registryClaimsHash(input.Network, id, input.Owner, input.Wallet), nil
}
func validRegistryBinding(o *RegistryObservations, v *Verification, e Entry) bool {
	if o == nil {
		return true
	}
	if e.Product != "agent_registry_check" || !validRegistryObservations(o) || v == nil || o.EvidenceSHA256 != v.EvidenceSHA256 || !configFingerprint.MatchString(e.RegistryRequestSHA256) {
		return false
	}
	if v.Block != nil && o.Network != "eip155:"+strconv.FormatInt(v.Block.ChainID, 10) {
		return false
	}
	return e.RegistryRequestSHA256 == registryClaimsHash(o.Network, o.AgentID, o.AddressClaims.Owner.Claim, o.AddressClaims.Wallet.Claim)
}
