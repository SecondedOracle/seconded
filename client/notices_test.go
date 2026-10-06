package client

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpdateNoticeRecoversInvalidState(t *testing.T) {
	now := time.Unix(1800000000, 0)
	for _, state := range []string{`{"last":`, `{"last":1800000001}`, `{"last":9223372036854775807}`} {
		t.Run(state, func(t *testing.T) {
			files, err := OpenFiles(filepath.Join(t.TempDir(), "profile"))
			if err != nil {
				t.Fatal(err)
			}
			if err := files.Write("update-notice.json", []byte(state)); err != nil {
				t.Fatal(err)
			}
			g := &Engine{Files: files}
			if got := g.takeUpdateNotice(now); got != UpdateAvailableNotice {
				t.Fatal("invalid state suppressed notice", got)
			}
			if got := g.takeUpdateNotice(now); got != "" {
				t.Fatal("fresh state did not suppress duplicate", got)
			}
		})
	}
}

func TestUpdateNoticeUnattachableResultDoesNotConsume(t *testing.T) {
	for _, result := range []any{nil, []string{"result"}, "result", make(chan int)} {
		files, err := OpenFiles(filepath.Join(t.TempDir(), "profile"))
		if err != nil {
			t.Fatal(err)
		}
		g := &Engine{Files: files}
		now := time.Unix(1800000000, 0)
		g.attachUpdateNotice(result, now)
		if got := g.attachUpdateNotice(map[string]any{"ok": true}, now); !strings.Contains(string(mustNoticeJSON(t, got)), UpdateAvailableNotice) {
			t.Fatalf("unattachable %T consumed notice", result)
		}
		if got := g.attachUpdateNotice(map[string]any{"ok": true}, now); strings.Contains(string(mustNoticeJSON(t, got)), UpdateAvailableNotice) {
			t.Fatal("attached notice did not consume daily slot")
		}
	}
}

func mustNoticeJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestUpdateNoticePreservesLargeNumbers(t *testing.T) {
	files, err := OpenFiles(filepath.Join(t.TempDir(), "profile"))
	if err != nil {
		t.Fatal(err)
	}
	g := &Engine{Files: files}
	value := map[string]any{"integer": uint64(18446744073709551615), "nested": map[string]any{"integer": int64(9007199254740993)}}
	got := string(mustNoticeJSON(t, g.attachUpdateNotice(value, time.Unix(1800000000, 0))))
	if !strings.Contains(got, UpdateAvailableNotice) || !strings.Contains(got, "18446744073709551615") || !strings.Contains(got, "9007199254740993") {
		t.Fatal("notice rounded tool data", got)
	}
}
