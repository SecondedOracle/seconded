package client

import (
	"context"
	"os"
	"strconv"
	"time"
)

const defaultToolSoftDeadline = 45 * time.Second

// This bounds local waiting, not the lifetime of an admitted server-side check.
func toolSoftDeadline() time.Duration {
	seconds, err := strconv.ParseFloat(os.Getenv("SECONDED_TOOL_SOFT_DEADLINE_SECONDS"), 64)
	if err != nil || !(seconds >= 0.001 && seconds <= 3600) {
		return defaultToolSoftDeadline
	}
	return time.Duration(seconds * float64(time.Second))
}

func checkContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, toolSoftDeadline())
}

// A credit quote includes identity verification and guarded server pricing,
// after any startup/profile contention, so it needs the network-work budget.
func toolTimeout(name string) time.Duration {
	if name == "seconded_receipt" || name == "seconded_credits_quote" || toolProduct(name) != "" {
		return toolSoftDeadline()
	}
	return 10 * time.Second
}
