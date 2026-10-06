package client

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"

	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
)

func keccak(parts ...[]byte) []byte {
	h := sha3.NewLegacyKeccak256()
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}
func word(v int64) []byte { return new(big.Int).SetInt64(v).FillBytes(make([]byte, 32)) }
func addressWord(s string) []byte {
	b, _ := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	r := make([]byte, 32)
	copy(r[12:], b)
	return r
}
func tokenDomain() []byte { return tokenDomainFor(Network) }
func tokenDomainFor(network string) []byte {
	p := pins(network)
	return keccak(keccak([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)")), keccak([]byte(p.Name)), keccak([]byte(p.Version)), word(p.ChainID), addressWord(p.Asset))
}

type Authorization struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Value       string `json:"value"`
	ValidAfter  string `json:"validAfter"`
	ValidBefore string `json:"validBefore"`
	Nonce       string `json:"nonce"`
}

func (a Authorization) Digest() ([]byte, error) { return a.DigestFor(Network) }
func (a Authorization) DigestFor(network string) ([]byte, error) {
	return a.digestFor(network, ValiditySeconds)
}

// Standard offers allow 380 seconds plus published clock tolerance. Keep the
// original-door 300-second bound separate so rollback policy does not loosen.
func (a Authorization) standardDigestFor(network string) ([]byte, error) {
	return a.digestFor(network, 411)
}
func (a Authorization) digestFor(network string, maxWindow int64) ([]byte, error) {
	if pins(network).Network == "" {
		return nil, ErrInvalid
	}
	v, e := atomic(a.Value)
	if e != nil || v <= 0 || v > MaxAuthorization || !addressPattern.MatchString(a.From) || strings.ToLower(a.To) != PayTo || !hexWord.MatchString(a.Nonce) {
		return nil, ErrInvalid
	}
	after, e := atomic(a.ValidAfter)
	if e != nil {
		return nil, e
	}
	before, e := atomic(a.ValidBefore)
	if e != nil || before <= after || before-after > maxWindow {
		return nil, ErrInvalid
	}
	nonce, _ := hex.DecodeString(a.Nonce[2:])
	h := keccak(keccak([]byte("TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)")), addressWord(a.From), addressWord(PayTo), word(v), word(after), word(before), nonce)
	return keccak([]byte{0x19, 0x01}, tokenDomainFor(network), h), nil
}

type Signer interface {
	Address() string
	SignDigest([]byte) ([]byte, error)
}
type CoinbaseLink interface{ Link() (Signer, error) }
type CoinbaseStub struct{}

func (CoinbaseStub) Link() (Signer, error) { return nil, errors.New("coinbase_link_not_implemented") }

type LocalSigner struct{ key *secp.PrivateKey }

func NewLocalSigner(raw []byte) (*LocalSigner, error) {
	if len(raw) != 32 {
		return nil, ErrInvalid
	}
	n := new(big.Int).SetBytes(raw)
	if n.Sign() == 0 || n.Cmp(secp.S256().Params().N) >= 0 {
		return nil, ErrInvalid
	}
	return &LocalSigner{secp.PrivKeyFromBytes(raw)}, nil
}
func GenerateKey() ([]byte, error) {
	k, e := secp.GeneratePrivateKeyFromRand(rand.Reader)
	if e != nil {
		return nil, errors.New("key_generation_failed")
	}
	defer k.Zero()
	return k.Serialize(), nil
}
func (s *LocalSigner) Address() string {
	p := s.key.PubKey().SerializeUncompressed()
	h := keccak(p[1:])
	return "0x" + hex.EncodeToString(h[12:])
}
func (s *LocalSigner) Close() { s.key.Zero() }
func (s *LocalSigner) SignDigest(d []byte) ([]byte, error) {
	if len(d) != 32 {
		return nil, ErrInvalid
	}
	c := ecdsa.SignCompact(s.key, d, false)
	out := append([]byte{}, c[1:]...)
	out = append(out, c[0])
	return out, nil
}
func signChecked(s Signer, d []byte) (string, error) {
	sig, e := s.SignDigest(d)
	if e != nil || len(sig) != 65 {
		return "", errors.New("signing_failed")
	}
	if sig[64] != 27 && sig[64] != 28 {
		return "", ErrInvalid
	}
	half := new(big.Int).Rsh(new(big.Int).Set(secp.S256().Params().N), 1)
	if new(big.Int).SetBytes(sig[32:64]).Cmp(half) > 0 {
		return "", ErrInvalid
	}
	compact := append([]byte{sig[64]}, sig[:64]...)
	pub, _, e := ecdsa.RecoverCompact(compact, d)
	if e != nil {
		return "", ErrInvalid
	}
	h := keccak(pub.SerializeUncompressed()[1:])
	if "0x"+hex.EncodeToString(h[12:]) != s.Address() {
		return "", ErrInvalid
	}
	return "0x" + hex.EncodeToString(sig), nil
}

func ownershipDigest(checkID, nonce string, expires int64) ([]byte, error) {
	proof := NewOwnershipProof(ChainID, checkID, nonce, expires)
	return proof.Digest()
}

// The ownership domain is independent of the token's transfer domain.
// Network selection is validated by the caller against the stored check.

// Transfer components are exposed internally for the versioned parity fixtures.
func (a Authorization) hashes() ([]byte, []byte, error) {
	if _, err := a.Digest(); err != nil {
		return nil, nil, err
	}
	value, _ := atomic(a.Value)
	after, _ := atomic(a.ValidAfter)
	before, _ := atomic(a.ValidBefore)
	nonce, _ := hex.DecodeString(a.Nonce[2:])
	structure := keccak(keccak([]byte("TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)")), addressWord(a.From), addressWord(a.To), word(value), word(after), word(before), nonce)
	return tokenDomain(), structure, nil
}

// verifyPaymentSigner validates a retained credential without signing again.
func verifyPaymentSigner(digest, sig []byte, payer string) error {
	if len(sig) != 65 || (sig[64] != 27 && sig[64] != 28) {
		return ErrInvalid
	}
	half := new(big.Int).Rsh(new(big.Int).Set(secp.S256().Params().N), 1)
	if new(big.Int).SetBytes(sig[32:64]).Cmp(half) > 0 {
		return ErrInvalid
	}
	pub, _, err := ecdsa.RecoverCompact(append([]byte{sig[64]}, sig[:64]...), digest)
	if err != nil {
		return ErrInvalid
	}
	hash := keccak(pub.SerializeUncompressed()[1:])
	if "0x"+hex.EncodeToString(hash[12:]) != payer {
		return ErrInvalid
	}
	return nil
}
