package client

import (
	"encoding/json"
	"reflect"
)

const OwnershipType = "OwnershipProof(string purpose,string check_id,string nonce,uint256 expires_at)"

// Purpose and validity are generated from the server contract; see ownership_contract.go.

type OwnershipDomain struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	ChainID int64  `json:"chainId"`
}
type OwnershipMessage struct {
	Purpose string `json:"purpose"`
	CheckID string `json:"check_id"`
	Nonce   string `json:"nonce"`
	Expires int64  `json:"expires_at"`
}
type OwnershipField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func ownershipTypes() map[string][]OwnershipField {
	return map[string][]OwnershipField{
		"EIP712Domain":   {{"name", "string"}, {"version", "string"}, {"chainId", "uint256"}},
		"OwnershipProof": {{"purpose", "string"}, {"check_id", "string"}, {"nonce", "string"}, {"expires_at", "uint256"}},
	}
}

type OwnershipProof struct {
	Types       map[string][]OwnershipField `json:"types"`
	Domain      OwnershipDomain             `json:"domain"`
	PrimaryType string                      `json:"primaryType"`
	Message     OwnershipMessage            `json:"message"`
}
type OwnershipChallenge struct {
	Receipt   json.RawMessage `json:"receipt,omitempty"`
	Hints     Hints           `json:"hints"`
	Reason    string          `json:"reason,omitempty"`
	Nonce     string          `json:"nonce,omitempty"`
	Error     string          `json:"error"`
	CheckID   string          `json:"check_id"`
	Ownership OwnershipProof  `json:"ownership"`
}

func NewOwnershipProof(chain int64, check, nonce string, expires int64) OwnershipProof {
	return OwnershipProof{ownershipTypes(), OwnershipDomain{"SECONDED", "1", chain}, "OwnershipProof", OwnershipMessage{OwnershipPurpose, check, nonce, expires}}
}
func (p OwnershipProof) hashes() ([]byte, []byte, error) {
	m := p.Message
	if p.PrimaryType != "OwnershipProof" || !reflect.DeepEqual(p.Types, ownershipTypes()) || p.Domain.Name != "SECONDED" || p.Domain.Version != "1" || (p.Domain.ChainID != 84532 && p.Domain.ChainID != 8453 && p.Domain.ChainID != 5042002 && p.Domain.ChainID != 46630 && p.Domain.ChainID != 5042 && p.Domain.ChainID != 4663) || m.Purpose != OwnershipPurpose || !hexID.MatchString(m.CheckID) || !hexID.MatchString(m.Nonce) || m.Expires <= 0 || m.Expires > 9007199254740991 {
		return nil, nil, ErrInvalid
	}
	domain := keccak(keccak([]byte("EIP712Domain(string name,string version,uint256 chainId)")), keccak([]byte(p.Domain.Name)), keccak([]byte(p.Domain.Version)), word(p.Domain.ChainID))
	h := keccak(keccak([]byte(OwnershipType)), keccak([]byte(m.Purpose)), keccak([]byte(m.CheckID)), keccak([]byte(m.Nonce)), word(m.Expires))
	return domain, h, nil
}
func (p OwnershipProof) Digest() ([]byte, error) {
	domain, h, err := p.hashes()
	if err != nil {
		return nil, err
	}
	return keccak([]byte{0x19, 0x01}, domain, h), nil
}
func (c OwnershipChallenge) Validate(check string, chain, now int64) error {
	m := c.Ownership.Message
	if c.Error != "ownership_required" || c.CheckID != check || m.CheckID != check || c.Ownership.Domain.ChainID != chain || m.Expires <= now || m.Expires-now > OwnershipValiditySeconds || (c.Nonce != "" && c.Nonce != m.Nonce) {
		return ErrInvalid
	}
	_, err := c.Ownership.Digest()
	return err
}

type Remedy struct {
	ID          string `json:"id"`
	CheckID     string `json:"check_id,omitempty"`
	MaxTier     string `json:"max_tier,omitempty"`
	AvailableAt *int64 `json:"available_at,omitempty"`
}

func (r Remedy) validate() bool {
	switch r.ID {
	case "pay_withheld_result":
		return hexID.MatchString(r.CheckID) && r.MaxTier == "" && r.AvailableAt == nil
	case "recovery_check":
		return r.CheckID == "" && r.MaxTier == "small" && r.AvailableAt != nil && *r.AvailableAt > 0
	case "contact_support":
		return r.CheckID == "" && r.MaxTier == "" && r.AvailableAt == nil
	}
	return false
}
