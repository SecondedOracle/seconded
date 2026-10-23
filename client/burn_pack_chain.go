package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
)

const burnToken = "0x833a7fa750628b391c35bf384d7706652c7e8202"
const burnUSDG = "0x5fc5360d0400a0fd4f2af552add042d716f1d168"
const burnRouter = "0x8876789976decbfcbbbe364623c63652db8c0904"
const burnPermit2 = "0x000000000022d473030f116ddee9f6b43ac78ba3"
const burnQuoter = "0xe202bb8dd524ee9c5e679e5b5809f7a373a982ef"
const burnRouterSHA256 = "9b70267279e3d106d908cbd43927a251acd7a5e94cf34da431228c3c421c7dfb"
const burnPoolManager = "0x8366a39cc670b4001a1121b8f6a443a643e40951"

var burnPools = [][]byte{
	joinWords(addressWord("0x"+strings.Repeat("0", 40)), addressWord(burnUSDG), word(0x800000), word(10), addressWord("0x06a889870c8f83640d6816319f72e2aa579b6080")),
	joinWords(addressWord("0x"+strings.Repeat("0", 40)), addressWord(burnToken), word(0), word(200), addressWord("0xe5e702641ea86f4ae6cc3cdaed2b886f976be044")),
}

func joinWords(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

type burnABI struct {
	data    []byte
	dynamic bool
}

func burnTuple(args ...burnABI) []byte {
	size := 0
	for _, a := range args {
		if a.dynamic {
			size += 32
		} else {
			size += len(a.data)
		}
	}
	var head, tail []byte
	for _, a := range args {
		if a.dynamic {
			head = append(head, word(int64(size+len(tail)))...)
			tail = append(tail, a.data...)
		} else {
			head = append(head, a.data...)
		}
	}
	return append(head, tail...)
}
func burnBlob(b []byte) []byte {
	out := append(word(int64(len(b))), b...)
	return append(out, make([]byte, (32-len(b)%32)%32)...)
}
func burnArray(parts ...[]byte) []byte {
	args := make([]burnABI, len(parts))
	for i, p := range parts {
		args[i] = burnABI{burnBlob(p), true}
	}
	return append(word(int64(len(parts))), burnTuple(args...)...)
}
func burnCall(signature string, args []byte) []byte {
	return append(keccak([]byte(signature))[:4], args...)
}
func burnSwapData(input int64, minimum *big.Int, deadline int64) []byte {
	single := func(pool int, amount []byte, min []byte) []byte {
		tuple := burnTuple(burnABI{burnPools[pool], false}, burnABI{word(int64(pool)), false}, burnABI{amount, false}, burnABI{min, false}, burnABI{word(0), false}, burnABI{burnBlob(nil), true})
		return append(word(32), tuple...)
	}
	first := single(0, word(input), word(0))
	second := single(1, word(0), minimum.FillBytes(make([]byte, 32)))
	settle := joinWords(addressWord(burnUSDG), word(input))
	take := joinWords(addressWord(burnToken), minimum.FillBytes(make([]byte, 32)))
	plan := burnTuple(burnABI{burnBlob([]byte{6, 6, 12, 15}), true}, burnABI{burnArray(first, second, settle, take), true})
	return burnCall("execute(bytes,bytes[],uint256)", burnTuple(burnABI{burnBlob([]byte{16}), true}, burnABI{burnArray(plan), true}, burnABI{word(deadline), false}))
}
func burnRLP(data []byte, list bool) []byte {
	if !list && len(data) == 1 && data[0] < 128 {
		return data
	}
	prefix := byte(128)
	if list {
		prefix = 192
	}
	if len(data) < 56 {
		return append([]byte{prefix + byte(len(data))}, data...)
	}
	n := new(big.Int).SetInt64(int64(len(data))).Bytes()
	return append(append([]byte{prefix + 55 + byte(len(n))}, n...), data...)
}
func burnRLPList(parts ...[]byte) []byte { return burnRLP(joinWords(parts...), true) }
func signedBurnTransaction(signer Signer, nonce uint64, to string, data []byte, spec BurnPackSpec) (string, string, error) {
	fee, ok := burnInteger(spec.MaxFeeWei)
	if !ok {
		return "", "", ErrInvalid
	}
	tip, ok := burnInteger(spec.MaxPriorityWei)
	if !ok {
		return "", "", ErrInvalid
	}
	address, err := hex.DecodeString(trimHex(to))
	if err != nil || len(address) != 20 {
		return "", "", ErrInvalid
	}
	fields := [][]byte{burnRLP(big.NewInt(4663).Bytes(), false), burnRLP(new(big.Int).SetUint64(nonce).Bytes(), false), burnRLP(tip.Bytes(), false), burnRLP(fee.Bytes(), false), burnRLP(new(big.Int).SetUint64(spec.GasLimit).Bytes(), false), burnRLP(address, false), burnRLP(nil, false), burnRLP(data, false), burnRLP(nil, true)}
	digest := keccak(append([]byte{2}, burnRLPList(fields...)...))
	signature, err := signChecked(signer, digest)
	if err != nil {
		return "", "", err
	}
	sig, _ := hex.DecodeString(signature[2:])
	fields = append(fields, burnRLP(big.NewInt(int64(sig[64]-27)).Bytes(), false), burnRLP(new(big.Int).SetBytes(sig[:32]).Bytes(), false), burnRLP(new(big.Int).SetBytes(sig[32:64]).Bytes(), false))
	raw := append([]byte{2}, burnRLPList(fields...)...)
	return "0x" + hex.EncodeToString(raw), "0x" + hex.EncodeToString(keccak(raw)), nil
}

type BurnPackReceipt struct {
	BlockHash   string
	BlockNumber string
	Success     bool
	Final       bool
	Received    string
}
type BurnPackChain interface {
	Validate(context.Context, string) error
	Quote(context.Context, int64) (*big.Int, error)
	Nonce(context.Context, string) (uint64, error)
	Send(context.Context, string, string) error
	Receipt(context.Context, string, string) (*BurnPackReceipt, error)
}
type rpcBurnPack struct {
	rpc      *RPC
	verifier *RPC
}

func newBurnPackChain() (BurnPackChain, error) {
	if !deploymentPaymentNetwork("eip155:4663") {
		return nil, errors.New("burn_buy_unavailable_in_staging")
	}
	origin := "https://rpc.mainnet.chain.robinhood.com"
	h, err := pinnedHTTP(origin, nil)
	if err != nil {
		return nil, err
	}
	second := "https://robinhood-rpc.publicnode.com"
	verifierHTTP, err := pinnedHTTP(second, nil)
	if err != nil {
		return nil, err
	}
	return &rpcBurnPack{rpc: &RPC{network: "eip155:4663", url: origin, http: h}, verifier: &RPC{network: "eip155:4663", url: second, http: verifierHTTP}}, nil
}
func rpcHexInteger(raw string) (*big.Int, error) {
	if len(raw) < 3 || raw[:2] != "0x" || len(raw) > 66 {
		return nil, ErrInvalid
	}
	n, ok := new(big.Int).SetString(raw[2:], 16)
	if !ok || n.Sign() < 0 {
		return nil, ErrInvalid
	}
	return n, nil
}
func (p *rpcBurnPack) Validate(ctx context.Context, payer string) error {
	if p.verifier != nil {
		if err := (&rpcBurnPack{rpc: p.verifier}).Validate(ctx, payer); err != nil {
			return err
		}
	}
	var chain, code string
	if err := p.rpc.call(ctx, "eth_chainId", []any{}, &chain); err != nil {
		return err
	}
	if chain != "0x1237" {
		return ErrInvalid
	}
	if err := p.rpc.CheckEOA(ctx, payer); err != nil {
		return err
	}
	if err := p.rpc.call(ctx, "eth_getCode", []any{burnRouter, "latest"}, &code); err != nil {
		return err
	}
	raw, err := hex.DecodeString(trimHex(code))
	if err != nil {
		return ErrInvalid
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != burnRouterSHA256 {
		return errors.New("burn_router_code_mismatch")
	}
	return nil
}
func (p *rpcBurnPack) Quote(ctx context.Context, input int64) (*big.Int, error) {
	amount := big.NewInt(input)
	for i, pool := range burnPools {
		params := burnTuple(burnABI{pool, false}, burnABI{word(int64(i)), false}, burnABI{amount.FillBytes(make([]byte, 32)), false}, burnABI{burnBlob(nil), true})
		data := burnCall("quoteExactInputSingle(((address,address,uint24,int24,address),bool,uint128,bytes))", append(word(32), params...))
		var response string
		err := p.rpc.call(ctx, "eth_call", []any{map[string]string{"to": burnQuoter, "data": "0x" + hex.EncodeToString(data)}, "latest"}, &response)
		if err != nil {
			return nil, err
		}
		raw, err := hex.DecodeString(trimHex(response))
		if err != nil || len(raw) != 64 {
			return nil, ErrInvalid
		}
		amount = new(big.Int).SetBytes(raw[:32])
		if amount.Sign() <= 0 || amount.BitLen() > 128 {
			return nil, ErrInvalid
		}
	}
	return amount, nil
}
func (p *rpcBurnPack) Nonce(ctx context.Context, payer string) (uint64, error) {
	if p.rpc == nil || p.verifier == nil {
		return 0, ErrChainUnavailable
	}
	var agreed uint64
	for i, reader := range []*RPC{p.rpc, p.verifier} {
		var raw string
		if err := reader.call(ctx, "eth_getTransactionCount", []any{payer, "pending"}, &raw); err != nil {
			return 0, err
		}
		n, err := rpcHexInteger(raw)
		if err != nil || !n.IsUint64() {
			return 0, ErrInvalid
		}
		if i > 0 && agreed != n.Uint64() {
			return 0, errors.New("burn_pack_nonce_changed")
		}
		agreed = n.Uint64()
	}
	return agreed, nil
}
func (p *rpcBurnPack) Send(ctx context.Context, raw, expected string) error {
	var hash string
	_, err := p.rpc.callOnce(ctx, "eth_sendRawTransaction", []any{raw}, &hash)
	if err != nil {
		return err
	}
	if strings.ToLower(hash) != expected {
		return ErrInvalid
	}
	return nil
}
func (p *rpcBurnPack) Receipt(ctx context.Context, hash, payer string) (*BurnPackReceipt, error) {
	first, err := p.receiptOne(ctx, hash, payer)
	if err != nil {
		return nil, err
	}
	if p.verifier == nil {
		return nil, ErrInvalid
	}
	second, err := (&rpcBurnPack{rpc: p.verifier}).receiptOne(ctx, hash, payer)
	if err != nil {
		return nil, err
	}
	if first == nil || second == nil {
		return nil, nil
	}
	if *first != *second {
		return nil, errors.New("burn_receipt_disagreement")
	}
	return first, nil
}
func (p *rpcBurnPack) receiptOne(ctx context.Context, hash, payer string) (*BurnPackReceipt, error) {
	var raw json.RawMessage
	if err := p.receiptRaw(ctx, hash, &raw); err != nil {
		return nil, err
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var receipt struct {
		TransactionHash string `json:"transactionHash"`
		Status          string `json:"status"`
		Block           string `json:"blockNumber"`
		BlockHash       string `json:"blockHash"`
		Logs            []struct {
			Address string   `json:"address"`
			Topics  []string `json:"topics"`
			Data    string   `json:"data"`
			Removed bool     `json:"removed"`
		} `json:"logs"`
	}
	if json.Unmarshal(raw, &receipt) != nil || strings.ToLower(receipt.TransactionHash) != hash || !hexWord.MatchString(receipt.BlockHash) || !oneOf(receipt.Status, "0x0", "0x1") {
		return nil, ErrInvalid
	}
	block, err := rpcHexInteger(receipt.Block)
	if err != nil || !block.IsUint64() {
		return nil, ErrInvalid
	}
	var head string
	if err = p.rpc.call(ctx, "eth_blockNumber", []any{}, &head); err != nil {
		return nil, err
	}
	number, err := rpcHexInteger(head)
	if err != nil {
		return nil, err
	}
	var blockRaw json.RawMessage
	var observed struct {
		Hash string `json:"hash"`
	}
	if err = p.rpc.call(ctx, "eth_getBlockByNumber", []any{receipt.Block, false}, &blockRaw); err != nil {
		return nil, err
	}
	if json.Unmarshal(blockRaw, &observed) != nil || observed.Hash != receipt.BlockHash {
		return nil, errors.New("burn_receipt_reorg")
	}
	received := new(big.Int)
	topic := "0x" + hex.EncodeToString(keccak([]byte("Transfer(address,address,uint256)")))
	receiver := "0x" + hex.EncodeToString(addressWord(payer))
	for _, log := range receipt.Logs {
		if strings.ToLower(log.Address) != burnToken || len(log.Topics) != 3 || log.Topics[0] != topic {
			continue
		}
		if log.Removed {
			return nil, ErrInvalid
		}
		value, err := rpcHexInteger(log.Data)
		if err != nil {
			return nil, err
		}
		if strings.ToLower(log.Topics[2]) == receiver {
			received.Add(received, value)
		}
		if strings.ToLower(log.Topics[1]) == receiver {
			received.Sub(received, value)
		}
	}
	return &BurnPackReceipt{BlockHash: receipt.BlockHash, BlockNumber: receipt.Block, Success: receipt.Status == "0x1", Final: new(big.Int).Sub(number, block).Cmp(big.NewInt(10)) >= 0, Received: received.String()}, nil
}

func burnPackTransaction(stage int, spec BurnPackSpec, quote BurnQuote, amount *big.Int) (string, []byte) {
	switch stage {
	case 0:
		return burnUSDG, burnCall("approve(address,uint256)", joinWords(addressWord(burnPermit2), word(spec.InputMicros)))
	case 1:
		return burnPermit2, burnCall("approve(address,address,uint160,uint48)", joinWords(addressWord(burnUSDG), addressWord(burnRouter), word(spec.InputMicros), word(quote.ExpiresAt)))
	case 2:
		return burnRouter, burnSwapData(spec.InputMicros, amount, quote.ExpiresAt)
	default:
		return burnToken, burnCall("burn(uint256)", amount.FillBytes(make([]byte, 32)))
	}
}

// The shared evidence RPC rejects null results; transaction polling must retain
// null as pending, without treating any other malformed response as nonpayment.
func (p *rpcBurnPack) receiptRaw(ctx context.Context, hash string, result *json.RawMessage) error {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "eth_getTransactionReceipt", "params": []any{hash}})
	request, err := http.NewRequestWithContext(ctx, "POST", p.rpc.url, jsonBody(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := p.rpc.http.Do(request)
	if err != nil {
		return err
	}
	raw, err := readResponse(response)
	if err != nil {
		return err
	}
	var envelope struct {
		Version string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
	}
	if response.StatusCode != 200 || DecodeStrict(raw, &envelope, ResponseLimit) != nil || envelope.Version != "2.0" || envelope.ID != 1 || len(envelope.Result) == 0 {
		return ErrInvalid
	}
	*result = envelope.Result
	return nil
}
