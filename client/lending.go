package client

import (
	"fmt"
	"time"
)

func lendingText(o *LendingObservations, now int64) ([]string, bool) {
	f := o.Sheet.Facts
	until := f.Source.Timestamp + f.Coverage.MaxAge
	expired := now > until
	state := "Observation valid until "
	if expired {
		state = "Historical observation expired at "
	}
	return []string{
		fmt.Sprintf("Morpho %s account %s, market %s: %s at block %d.", f.Network, f.Account, f.MarketID, f.Position.Relationship, f.Source.BlockNumber),
		fmt.Sprintf("Supply shares %s; borrow shares %s; collateral %s atomic units.", f.Position.SupplyShares, f.Position.BorrowShares, f.Position.Collateral),
		state + time.Unix(until, 0).UTC().Format(time.RFC3339) + ".",
		"Coverage: one account and one market. Oracle accuracy, oracle feed age, other markets and future liquidation safety are not checked.",
	}, expired
}
