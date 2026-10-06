package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	syncatomic "sync/atomic"
	"time"
)

const UpdateAvailableNotice = "A newer SECONDED release is available; ask your owner to update."

type updateSignalKey struct{}

func observeLatestVersion(ctx context.Context, headers http.Header) {
	signal, _ := ctx.Value(updateSignalKey{}).(*syncatomic.Bool)
	versions := headers.Values("SECONDED-LATEST-CLIENT-VERSION")
	if signal != nil && len(versions) == 1 && newerVersion(versions[0], Version) {
		signal.Store(true)
	}
}

// Failure to persist an advisory suppresses it, never the tool result. The
// profile lock makes the rolling 24-hour limit shared across hosts and restarts.
func (g *Engine) takeUpdateNotice(now time.Time) string {
	if g == nil || g.Files == nil {
		return ""
	}
	unlock, err := g.Files.Lock()
	if err != nil {
		return ""
	}
	defer unlock()
	var state struct {
		Last int64 `json:"last"`
	}
	raw, err := g.Files.Read("update-notice.json")
	if err == nil {
		if DecodeStrict(raw, &state, 1024) == nil && state.Last <= now.Unix() && state.Last > now.Unix()-86400 {
			return ""
		}
	} else if !errors.Is(err, ErrNotFound) {
		return ""
	}
	state.Last = now.Unix()
	raw, _ = json.Marshal(state)
	if g.Files.Write("update-notice.json", raw) != nil {
		return ""
	}
	return UpdateAvailableNotice
}

func noticeMessages(codes []string) []string {
	messages := make([]string, 0, len(codes))
	for _, code := range codes {
		switch code {
		case "receipt_ledger_mismatch":
			messages = append(messages, "Your answer is verified. Payment confirmation is still pending, so its amount remains reserved. Call seconded_receipt again; do not submit another payment.")
		case "server_refund_owed":
			messages = append(messages, "A refund is due. Call seconded_receipt to check its progress.")
		case "server_refunded":
			messages = append(messages, "The service reports your refund was sent. Check seconded_wallet for the returned funds.")
		default:
			window, pct, ok := strings.Cut(code, "_")
			label := map[string]string{"hour": "hourly", "day": "daily", "outstanding": "pending-payment"}[window]
			if ok && label != "" && oneOf(pct, "50", "80", "100") {
				messages = append(messages, "You have used or reserved at least "+pct+"% of your "+label+" allowance. Check seconded_get_limits before starting more checks.")
			} else {
				messages = append(messages, "Your wallet needs attention. Call seconded_wallet to check its status.")
			}
		}
	}
	return messages
}

func (r *Result) addNotices(codes ...string) {
	r.NoticeCodes = append(r.NoticeCodes, codes...)
	r.Notices = append(r.Notices, noticeMessages(codes)...)
}
