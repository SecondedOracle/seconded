package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RPC struct {
	network string
	server  bool // Optional API reader; never sufficient without a public reader.
	url     string
	http    *http.Client
}

// EvidenceEndpoint is one compiled URL/operator member of the v3.1 E-set.
// Web PKI validates its TLS chain and hostname, without pinning rotating keys.
type EvidenceEndpoint struct {
	URL      string
	Operator string
}

// The E-set matches the server's READ_RPCS. Both independent readers must agree
// on finalized facts. URLs/operators are release-review inputs, with no runtime
// override, proxy, redirect or server-supplied replacement.
var releaseEvidence = []EvidenceEndpoint{
	{URL: "https://sepolia.base.org", Operator: "coinbase"},
	{URL: "https://base-sepolia-rpc.publicnode.com", Operator: "publicnode"},
}

var errEvidenceNotConfigured = errors.New("evidence_identity_not_configured")
var errEvidenceDisagreement = errors.New("chain_evidence_disagreement")

// ErrChainUnavailable is retryable and never certifies a payment outcome.
var ErrChainUnavailable = errors.New("chain_unavailable")

// ErrReaderRateLimited identifies an unavailable reader whose quota is exhausted.
var ErrReaderRateLimited error = readerRateLimitError{}

type readerRateLimitError struct{}

func (readerRateLimitError) Error() string { return "chain_rate_limited" }
func (readerRateLimitError) Unwrap() error { return ErrChainUnavailable }

// ErrReaderCapabilityUnavailable permits a different independent pair without
// repeating a request that exceeds this reader's capabilities.
var ErrReaderCapabilityUnavailable error = readerCapabilityError{}

type readerCapabilityError struct{}

func (readerCapabilityError) Error() string { return "chain_reader_capability_unavailable" }
func (readerCapabilityError) Unwrap() error { return ErrChainUnavailable }

// ErrReaderHistoryUnavailable leaves reconciliation retryable without treating
// an archive access policy as malformed payment evidence.
var ErrReaderHistoryUnavailable error = readerHistoryError{}

type readerHistoryError struct{}

func (readerHistoryError) Error() string { return "chain_reader_history_unavailable" }
func (readerHistoryError) Unwrap() error { return ErrChainUnavailable }

func init() {
	publicErrorMessages[ErrReaderCapabilityUnavailable.Error()] = "A chain reader cannot serve this request. Retry later; payment status is not yet known."
	publicErrorMessages[ErrReaderRateLimited.Error()] = "A chain reader is rate-limited. Retry later; payment status is not yet known."
	publicErrorMessages[ErrReaderHistoryUnavailable.Error()] = "A chain reader cannot serve the required history. Retry later; payment status is not yet known and funds remain reserved. If this persists, update the client or contact support."
}

const rpcRetryBudget = 4 * time.Second
const rpcMaxAttempts = 4

// Leave time to retry a stalled public reader inside the overall call budget.
const rpcAttemptBudget = 2 * time.Second

func waitRPC(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil || errors.Is(err, strconv.ErrRange) {
		const maxDelay = time.Duration(1<<63 - 1)
		if seconds > uint64(maxDelay/time.Second) {
			return maxDelay
		}
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date.Sub(now)
	}
	return 0
}

func ReleaseEvidence() (*Quorum, error) { return ReleaseEvidenceFor(Network) }

func newQuorum(endpoints []EvidenceEndpoint) (*Quorum, error) {
	if len(endpoints) < 2 {
		return nil, errEvidenceNotConfigured
	}
	operators, hosts := map[string]bool{}, map[string]bool{}
	q := &Quorum{}
	for _, p := range endpoints {
		u, e := url.Parse(p.URL)
		if e != nil || p.Operator == "" || operators[p.Operator] || hosts[strings.ToLower(u.Hostname())] {
			return nil, errEvidenceNotConfigured
		}
		operators[p.Operator], hosts[strings.ToLower(u.Hostname())] = true, true
		h, e := pinnedHTTP(p.URL, nil)
		if e != nil {
			return nil, e
		}
		q.rpcs = append(q.rpcs, &RPC{url: p.URL, http: h})
	}
	return q, nil
}

// Quorum selects a two-reader certificate from independent release pins.
// Unavailability permits another pair; disagreement fails closed. A server
// reader must be cross-checked against a public reader for every certificate.
type Quorum struct{ rpcs []*RPC }

func (q *Quorum) Snapshot(ctx context.Context, payer string) (ChainSnapshot, error) {
	if len(q.rpcs) > 2 {
		return quorumPair(q, func(pair *Quorum) (ChainSnapshot, error) { return pair.Snapshot(ctx, payer) })
	}
	if !q.publicPair() {
		return ChainSnapshot{}, errEvidenceNotConfigured
	}
	if recentStateNetwork(q.rpcs[0].network) {
		return q.recentSnapshot(ctx, payer)
	}
	snaps := make([]ChainSnapshot, len(q.rpcs))
	errs := make([]error, len(q.rpcs))
	var readers sync.WaitGroup
	for i, r := range q.rpcs {
		readers.Add(1)
		go func() {
			defer readers.Done()
			snaps[i], errs[i] = r.Snapshot(ctx, payer)
		}()
	}
	readers.Wait()
	if err := evidenceReadFailure(errs); err != nil {
		return ChainSnapshot{}, err
	}
	// Finalized heads advance independently, so compare at the lowest head.
	low := snaps[0].Number
	for _, s := range snaps[1:] {
		low = min(low, s.Number)
	}
	for i, r := range q.rpcs {
		if snaps[i].Number != low {
			s, e := r.snapshotAt(ctx, payer, fmt.Sprintf("0x%x", low))
			if e != nil {
				return ChainSnapshot{}, e
			}
			snaps[i] = s
		}
		if snaps[i] != snaps[0] {
			return ChainSnapshot{}, errEvidenceDisagreement
		}
	}
	return snaps[0], nil
}

func (q *Quorum) Evidence(ctx context.Context, e Entry, s ChainSnapshot) (ChainEvidence, error) {
	if len(q.rpcs) > 2 {
		return quorumPair(q, func(pair *Quorum) (ChainEvidence, error) { return pair.Evidence(ctx, e, s) })
	}
	if !q.publicPair() {
		return ChainEvidence{}, errEvidenceNotConfigured
	}
	evidence := make([]ChainEvidence, len(q.rpcs))
	errs := make([]error, len(q.rpcs))
	var readers sync.WaitGroup
	for i, r := range q.rpcs {
		// Each endpoint reads at the agreed block hash with requireCanonical.
		readers.Add(1)
		go func() {
			defer readers.Done()
			evidence[i], errs[i] = r.Evidence(ctx, e, s)
		}()
	}
	readers.Wait()
	if err := evidenceReadFailure(errs); err != nil && !errors.Is(err, ErrChainUnavailable) {
		return ChainEvidence{}, err
	}
	for _, err := range errs {
		if errors.Is(err, ErrReaderHistoryUnavailable) {
			return q.receiptFallback(ctx, e, s, evidence, errs)
		}
	}
	for _, err := range errs {
		if err != nil {
			return ChainEvidence{}, err
		}
	}
	first := evidence[0]
	for _, ev := range evidence[1:] {
		if ev != first {
			return ChainEvidence{}, errEvidenceDisagreement
		}
	}
	if err := q.recheckRecentSnapshot(ctx, s); err != nil {
		return ChainEvidence{}, err
	}
	return first, nil
}
func (r *RPC) call(ctx context.Context, method string, params any, dst any) (err error) {
	// The caller's reconciliation deadline always wins over this per-call budget.
	ctx, cancel := context.WithTimeout(ctx, rpcRetryBudget)
	defer cancel()
	defer func() {
		if err != nil {
			if errors.Is(err, ErrChainUnavailable) && !errors.Is(err, ErrReaderRateLimited) && errors.Is(ctx.Err(), context.DeadlineExceeded) {
				err = errReaderTimeout
			}
			err = &rpcReadError{reader: r, method: method, cause: err}
		}
	}()
	limited := false
	last := ErrChainUnavailable
	for attempt := 0; attempt < rpcMaxAttempts; attempt++ {
		if ctx.Err() != nil {
			if limited {
				return ErrReaderRateLimited
			}
			return ErrChainUnavailable
		}
		deadline, _ := ctx.Deadline()
		remaining := time.Until(deadline)
		budget := min(remaining, max(rpcAttemptBudget, remaining/time.Duration(rpcMaxAttempts-attempt)))
		attemptCtx, attemptCancel := context.WithTimeout(ctx, budget)
		after, err := r.callOnce(attemptCtx, method, params, dst)
		if errors.Is(attemptCtx.Err(), context.DeadlineExceeded) && errors.Is(err, ErrChainUnavailable) {
			err = errReaderTimeout
		}
		attemptCancel()
		// Retry only the bare availability sentinel: wrapped history/policy errors
		// also unwrap to it, but repeating them cannot repair the evidence.
		if err != ErrChainUnavailable && err != errReaderTimeout && !errors.Is(err, ErrReaderRateLimited) {
			if limited && ctx.Err() != nil && errors.Is(err, ErrChainUnavailable) {
				return ErrReaderRateLimited
			}
			return err
		}
		limited = errors.Is(err, ErrReaderRateLimited)
		last = err
		if attempt+1 == rpcMaxAttempts {
			break
		}
		backoff := 250 * time.Millisecond * time.Duration(1<<attempt)
		delay := max(after, backoff+time.Duration(rand.Int64N(int64(backoff/2))))
		// Never shorten Retry-After to fit the budget and send an early request.
		if deadline, ok := ctx.Deadline(); ok && delay >= time.Until(deadline) {
			break
		}
		if waitRPC(ctx, delay) != nil {
			break
		}
	}
	return last
}

func (r *RPC) callOnce(ctx context.Context, method string, params any, dst any) (time.Duration, error) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, e := http.NewRequestWithContext(ctx, "POST", r.url, jsonBody(b))
	if e != nil {
		return 0, ErrInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "seconded/"+ClientVersion)
	resp, e := r.http.Do(req)
	if e != nil {
		return 0, ErrChainUnavailable
	}
	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		resp.Body.Close()
		return 0, ErrReaderCapabilityUnavailable
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		return retryAfter(resp.Header.Get("Retry-After"), time.Now()), ErrReaderRateLimited
	}
	if resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout {
		resp.Body.Close()
		return retryAfter(resp.Header.Get("Retry-After"), time.Now()), ErrChainUnavailable
	}
	b, e = readResponse(resp)
	if e != nil {
		if ctx.Err() != nil {
			return 0, ErrChainUnavailable
		}
		return 0, ErrInvalid
	}
	var env struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   json.RawMessage `json:"error,omitempty"`
	}
	if DecodeStrict(b, &env, ResponseLimit) != nil || env.JSONRPC != "2.0" || env.ID != 1 {
		return 0, ErrInvalid
	}
	if len(env.Error) > 0 {
		var rpcError struct {
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data,omitempty"`
		}
		if (len(env.Result) == 0 || string(env.Result) == "null") && DecodeStrict(env.Error, &rpcError, ResponseLimit) == nil {
			if rpcError.Code == -32601 || rpcError.Code == -32004 ||
				(method == "eth_getLogs" && readerRangeLimit(rpcError.Code, rpcError.Message)) {
				return 0, ErrReaderCapabilityUnavailable
			}
			if rpcError.Code == -32005 || (rpcError.Code == -32016 && strings.EqualFold(strings.TrimSpace(rpcError.Message), "over rate limit")) {
				return retryAfter(resp.Header.Get("Retry-After"), time.Now()), ErrReaderRateLimited
			}
			if resp.StatusCode == http.StatusForbidden && rpcError.Code == -32602 &&
				strings.HasPrefix(strings.ToLower(rpcError.Message), "archive requests require a personal token") {
				// Retrying the identical request cannot change an archive policy.
				return 0, ErrReaderHistoryUnavailable
			}
		}
		return 0, ErrInvalid
	}
	if resp.StatusCode != http.StatusOK || len(env.Result) == 0 || string(env.Result) == "null" {
		return 0, ErrInvalid
	}
	return 0, DecodeStrict(env.Result, dst, ResponseLimit)
}

var quantityPattern = regexp.MustCompile(`^0x(0|[1-9a-f][0-9a-f]*)$`)

func quantity(s string) (uint64, error) {
	if !quantityPattern.MatchString(s) {
		return 0, ErrInvalid
	}
	v, e := strconv.ParseUint(s[2:], 16, 64)
	if e != nil {
		return 0, ErrInvalid
	}
	return v, nil
}
func (r *RPC) uintCall(ctx context.Context, data, blockHash string) (*big.Int, error) {
	var out string
	param := map[string]any{"blockHash": blockHash, "requireCanonical": true}
	if e := r.call(ctx, "eth_call", []any{map[string]string{"to": pins(r.network).Asset, "data": data}, param}, &out); e != nil {
		return nil, e
	}
	if !hexWord.MatchString(out) {
		return nil, ErrInvalid
	}
	v, ok := new(big.Int).SetString(out[2:], 16)
	if !ok {
		return nil, ErrInvalid
	}
	return v, nil
}
func calldata(sig string, args ...[]byte) string {
	b := append([]byte{}, keccak([]byte(sig))[:4]...)
	for _, a := range args {
		b = append(b, a...)
	}
	return "0x" + hex.EncodeToString(b)
}

// Ethereum blocks contain chain-defined extensible fields. Each permitted field
// is explicitly enumerated; the three evidence fields are validated separately.
func (r *RPC) block(ctx context.Context, tag string) (ChainSnapshot, error) {
	var obj map[string]json.RawMessage
	var s ChainSnapshot
	if e := r.call(ctx, "eth_getBlockByNumber", []any{tag, false}, &obj); e != nil {
		return s, e
	}
	allowed := strings.Fields("number hash parentHash nonce sha3Uncles logsBloom transactionsRoot stateRoot receiptsRoot miner difficulty totalDifficulty extraData size gasLimit gasUsed timestamp transactions uncles baseFeePerGas mixHash withdrawals withdrawalsRoot blobGasUsed excessBlobGas parentBeaconBlockRoot requestsHash")
	allowed = append(allowed, pins(r.network).BlockFields...)
	for k := range obj {
		ok := false
		for _, a := range allowed {
			if k == a {
				ok = true
				break
			}
		}
		if !ok {
			return s, ErrInvalid
		}
	}
	var n, t string
	if json.Unmarshal(obj["number"], &n) != nil || json.Unmarshal(obj["hash"], &s.Hash) != nil || json.Unmarshal(obj["timestamp"], &t) != nil || !hexWord.MatchString(s.Hash) {
		return s, ErrInvalid
	}
	var e error
	s.Number, e = quantity(n)
	if e != nil {
		return s, e
	}
	ts, e := quantity(t)
	if e != nil || ts > 1<<63-1 {
		return s, ErrInvalid
	}
	s.Timestamp = int64(ts)
	return s, nil
}
func (r *RPC) Snapshot(ctx context.Context, payer string) (ChainSnapshot, error) {
	return r.snapshotAt(ctx, payer, "finalized")
}

// snapshotAt reads a tagged or numbered block; the caller selects its finality.
func (r *RPC) snapshotAt(ctx context.Context, payer, tag string) (ChainSnapshot, error) {
	var s ChainSnapshot
	if !addressPattern.MatchString(payer) {
		return s, ErrInvalid
	}
	var id string
	readID := func() error { return r.call(ctx, "eth_chainId", []any{}, &id) }
	readBlock := func() (err error) { s, err = r.block(ctx, tag); return }
	if recentStateNetwork(r.network) {
		if err := parallelReads(readID, readBlock); err != nil {
			return s, err
		}
	} else {
		if err := readID(); err != nil {
			return s, err
		}
		if err := readBlock(); err != nil {
			return s, err
		}
	}
	chain, e := quantity(id)
	if e != nil || chain != uint64(pins(r.network).ChainID) {
		return s, ErrInvalid
	}
	now := time.Now().Unix()
	if s.Timestamp > now+30 || s.Timestamp < now-pins(r.network).FinalizedMaxAge {
		return s, errors.New("chain_stale")
	}
	var d, domain, balance *big.Int
	reads := []func() error{
		func() (err error) { d, err = r.uintCall(ctx, calldata("decimals()"), s.Hash); return },
		func() (err error) { domain, err = r.uintCall(ctx, calldata("DOMAIN_SEPARATOR()"), s.Hash); return },
		func() (err error) {
			balance, err = r.uintCall(ctx, calldata("balanceOf(address)", addressWord(payer)), s.Hash)
			return
		},
	}
	if recentStateNetwork(r.network) {
		if err := parallelReads(reads...); err != nil {
			return s, err
		}
	} else {
		for _, read := range reads {
			if err := read(); err != nil {
				return s, err
			}
		}
	}
	if !d.IsInt64() || d.Int64() != 6 {
		return s, ErrInvalid
	}
	if domain.Cmp(new(big.Int).SetBytes(tokenDomainFor(r.network))) != 0 {
		return s, ErrInvalid
	}
	if !balance.IsInt64() {
		s.Balance = 1<<63 - 1
	} else {
		s.Balance = balance.Int64()
	}
	s.Network = normalizedNetwork(r.network)
	return s, nil
}

// Check the latest code immediately before signing, since delegation is per chain.
func (r *RPC) CheckEOA(ctx context.Context, payer string) error {
	var code string
	if err := r.call(ctx, "eth_getCode", []any{payer, "latest"}, &code); err != nil {
		return err
	}
	if code != "0x" {
		return errors.New("payer_has_code_eip7702_or_contract_wallet")
	}
	return nil
}
func (q *Quorum) CheckEOA(ctx context.Context, payer string) error {
	if len(q.rpcs) > 2 {
		_, err := quorumPair(q, func(pair *Quorum) (bool, error) { return true, pair.CheckEOA(ctx, payer) })
		return err
	}
	if !q.publicPair() {
		return errEvidenceNotConfigured
	}
	for _, r := range q.rpcs {
		if err := r.CheckEOA(ctx, payer); err != nil {
			return err
		}
	}
	return nil
}

// logTimestamp preserves strict wire typing while allowing older readers to omit it.
type logTimestamp string

func (v *logTimestamp) UnmarshalJSON(data []byte) error {
	var text string
	if json.Unmarshal(data, &text) != nil {
		return ErrInvalid
	}
	if _, err := quantity(text); err != nil {
		return err
	}
	*v = logTimestamp(text)
	return nil
}

type chainLog struct {
	BlockTimestamp logTimestamp `json:"blockTimestamp,omitempty"`
	Address        string       `json:"address"`
	Topics         []string     `json:"topics"`
	Data           string       `json:"data"`
	BlockNumber    string       `json:"blockNumber"`
	BlockHash      string       `json:"blockHash"`
	Tx             string       `json:"transactionHash"`
	TxIndex        string       `json:"transactionIndex"`
	Index          string       `json:"logIndex"`
	Removed        bool         `json:"removed"`
}

func (r *RPC) logs(ctx context.Context, from, to uint64, topics []any) ([]chainLog, error) {
	var logs []chainLog
	if e := r.call(ctx, "eth_getLogs", []any{map[string]any{"address": pins(r.network).Asset, "fromBlock": fmt.Sprintf("0x%x", from), "toBlock": fmt.Sprintf("0x%x", to), "topics": topics}}, &logs); e != nil {
		return nil, e
	}
	for _, l := range logs {

		n, e := quantity(l.BlockNumber)
		if e != nil || n < from || n > to || l.Removed || strings.ToLower(l.Address) != pins(r.network).Asset || !hexWord.MatchString(l.BlockHash) || !hexWord.MatchString(l.Tx) || len(l.Topics) > 4 {
			return nil, ErrInvalid
		}
		if _, e = quantity(l.Index); e != nil {
			return nil, e
		}
		if _, e = quantity(l.TxIndex); e != nil {
			return nil, e
		}
		for _, t := range l.Topics {
			if !hexWord.MatchString(t) {
				return nil, ErrInvalid
			}
		}
	}
	return logs, nil
}
func (r *RPC) Evidence(ctx context.Context, e Entry, s ChainSnapshot) (ChainEvidence, error) {
	out := ChainEvidence{State: "OUTSTANDING"}
	if normalizedNetwork(e.Network) != normalizedNetwork(r.network) {
		return out, ErrInvalid
	}
	nonce, _ := hex.DecodeString(e.Nonce[2:])
	stateHash := s.Hash
	if recentStateNetwork(r.network) {
		if !hexWord.MatchString(s.StateHash) || s.StateNumber < s.Number {
			return out, ErrInvalid
		}
		stateHash = s.StateHash
	}
	used, err := r.uintCall(ctx, calldata("authorizationState(address,bytes32)", addressWord(e.Payer), nonce), stateHash)
	if err != nil {
		return out, err
	}
	if !used.IsInt64() || (used.Int64() != 0 && used.Int64() != 1) {
		return out, ErrInvalid
	}
	out.authorizationUsed = used.Int64() == 1
	if used.Int64() == 0 {
		if s.Timestamp > e.ValidBefore {
			out.State = "RELEASED"
		}
		return out, nil
	}
	// A used bit alone could mean cancellation; require the matching event and,
	// for a payment, the token transfer in that same finalized transaction.
	usedTopic := "0x" + hex.EncodeToString(keccak([]byte("AuthorizationUsed(address,bytes32)")))
	cancelTopic := "0x" + hex.EncodeToString(keccak([]byte("AuthorizationCanceled(address,bytes32)")))
	payerTopic := "0x" + hex.EncodeToString(addressWord(e.Payer))
	if s.Number < e.StartBlock || s.Number-e.StartBlock > pins(r.network).HistoryBlocks {
		return out, errors.New("chain_history_unavailable")
	}
	var found *ChainEvidence
	chunk := uint64(1000)
	if baseRecentNetwork(r.network) {
		// The public Base reader rejects eth_getLogs ranges over 500 blocks.
		// Scan the same complete history in smaller, inclusive ranges.
		chunk = 500
	}
	arc := r.network == "eip155:5042002" || r.network == "eip155:5042"
	if arc {
		chunk = 10000
	}
	if limit := r.logRangeLimit(); limit != 0 {
		chunk = min(chunk, limit)
	}
	for start := e.StartBlock; start <= s.Number; {
		end := start + min(chunk-1, s.Number-start)
		topics := []any{[]string{usedTopic, cancelTopic}, payerTopic, e.Nonce}
		var logs []chainLog
		var err error
		if recentStateNetwork(r.network) {
			// Robinhood produces many blocks during L1 finality lag. Fetch at
			// most four disjoint ranges together, still verifying all history.
			end = start + min(4*chunk-1, s.Number-start)
			logs, err = r.recentLogs(ctx, start, end, chunk, topics)
		} else {
			logs, err = r.logs(ctx, start, end, topics)
		}
		if err != nil {
			return out, err
		}
		for _, l := range logs {
			if len(l.Topics) != 3 || l.Topics[1] != payerTopic || l.Topics[2] != e.Nonce || l.Data != "0x" {
				return out, ErrInvalid
			}
			block, err := r.block(ctx, l.BlockNumber)
			if err != nil {
				return out, err
			}
			if block.Hash != l.BlockHash || block.Timestamp > s.Timestamp {
				return out, ErrInvalid
			}
			if l.Topics[0] == cancelTopic {
				if found != nil {
					return out, ErrInvalid
				}
				found = &ChainEvidence{State: "RELEASED", authorizationUsed: true}
				continue
			}
			if l.Topics[0] != usedTopic {
				return out, ErrInvalid
			}
			num, _ := quantity(l.BlockNumber)
			transfer := "0x" + hex.EncodeToString(keccak([]byte("Transfer(address,address,uint256)")))
			to := "0x" + hex.EncodeToString(addressWord(PayTo))
			transfers, err := r.logs(ctx, num, num, []any{transfer, payerTopic, to})
			if err != nil {
				return out, err
			}
			matches := 0
			for _, tr := range transfers {
				if len(tr.Topics) != 3 || tr.Topics[0] != transfer || tr.Topics[1] != payerTopic || tr.Topics[2] != to || !hexWord.MatchString(tr.Data) {
					return out, ErrInvalid
				}
				amount, _ := new(big.Int).SetString(tr.Data[2:], 16)
				if tr.Tx == l.Tx && tr.BlockHash == block.Hash && amount.IsInt64() && amount.Int64() == e.Amount {
					matches++
				}
			}
			if matches != 1 {
				return out, ErrInvalid
			}
			if found != nil {
				return out, ErrInvalid
			}
			found = &ChainEvidence{State: "SPENT", SpentAt: block.Timestamp, Tx: l.Tx, authorizationUsed: true}
		}
		if end == s.Number {
			break
		}
		start = end + 1
		// call backs off only after a rate limit. Healthy scans must retain the
		// caller's budget for every range and both independent readers.
		if ctx.Err() != nil {
			return out, ErrChainUnavailable
		}
	}
	if found != nil {
		return *found, nil
	}
	return out, nil
}

// Match observed provider policy errors narrowly; unknown RPC errors stay invalid.
func readerRangeLimit(code int, message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	return (code == -32614 && regexp.MustCompile(`^eth_getlogs is limited to a [0-9]+ range$`).MatchString(message)) ||
		((code == 35 || code == -32602 || code == -32000) && regexp.MustCompile(`^ranges over [0-9]+ blocks are not supported on free plan$`).MatchString(message))
}

// Reviewed endpoint limits supplement conservative per-chain scan sizes.
func (r *RPC) logRangeLimit() uint64 {
	u, err := url.Parse(r.url)
	if err != nil {
		return 0
	}
	switch strings.ToLower(u.Hostname()) {
	case "mainnet.base.org", "sepolia.base.org":
		return 500
	case "base.drpc.org", "robinhood.drpc.org", "rpc.drpc.mainnet.arc.io":
		return 10000
	}
	return 0
}
