package client

import (
	"math/big"
	"strings"
	"time"
)

const privacyShieldedMaxAge int64 = 7 * 86400
const privacyShieldedNotice = "Pool balances are not eligible depositor counts or anonymity guarantees. Public amounts, timing, funding and bridge legs can remain linkable."
const privacyShieldedFormula = "shield = amount * 25 / 10000; unshield = (amount - shield) * 25 / 10000; total = shield + unshield"

var privacyRailgunContracts = map[string]string{
	"ethereum": "0xfa7093cdd9ee6932b4eb2c9e1cde7ce00b1fa4b9",
	"arbitrum": "0xfa7093cdd9ee6932b4eb2c9e1cde7ce00b1fa4b9",
	"polygon":  "0x19b620929f97b7b990801496c3b361ca5def8c71",
	"bsc":      "0x590162bf4b50f6576a459b75309ee21d92178a10",
}
var privacyPoolsContracts = privacyMap{
	"entrypoint_proxy":          "0x6818809eefce719e480a7526d76bd3e561526b46",
	"entrypoint_implementation": "0xdd8aa0560a08e39c0b3a84bba356bc025afbd4c1",
	"pool":                      "0xf241d57c6debae225c0f2e6ea1529373c9a9c9fb",
	"withdrawal_verifier":       "0x022891f938ae7fdc8ab9ead0fbf50aba8c897d6d",
	"ragequit_verifier":         "0xa45aca8604a73d80c551faad6355a5c3a5565ec6",
}

func privacyShieldedSnapshot(document privacyMap, now time.Time) (privacyMap, string) {
	if document == nil {
		document = privacyDocument("shielded-routes")
	}
	p := privacyObject(document["payload"])
	if p == nil || document["sha256"] != privacyHash(p) || p["schema"] != "seconded-shielded-snapshot/v1" || privacyInt(p["max_age_seconds"]) != privacyShieldedMaxAge || privacyInt(p["thin_crowd_threshold_bps"]) != 100 {
		return nil, "snapshot_unavailable"
	}
	fresh := func(v any) bool {
		age := privacyWallSeconds(now) - float64(privacyInt(v))
		return privacyInt(v) > 0 && age >= 0 && age <= float64(privacyShieldedMaxAge)
	}
	if !fresh(p["reviewed_at"]) {
		return nil, "stale_or_future_snapshot"
	}
	venues := privacyArray(p["venues"])
	if len(venues) != 5 {
		return nil, "snapshot_unavailable"
	}
	seen := map[string]bool{}
	for _, value := range venues {
		venue := privacyObject(value)
		id, chain := privacyString(venue["id"]), privacyString(venue["chain"])
		key := id + "/" + chain
		var expected privacyMap
		if id == "railgun" && privacyRailgunContracts[chain] != "" {
			expected = privacyMap{"pool": privacyRailgunContracts[chain]}
			if privacyInt(venue["fee_bps"]) != 25 {
				return nil, "snapshot_unavailable"
			}
		} else if id == "privacy_pools" && chain == "ethereum" {
			expected = privacyPoolsContracts
			if venue["fee_bps"] != nil {
				return nil, "snapshot_unavailable"
			}
		}
		if expected == nil || seen[key] || privacyHash(venue["contracts"]) != privacyHash(expected) {
			return nil, "snapshot_unavailable"
		}
		seen[key] = true
		assets := privacyObject(venue["assets"])
		expectedAssets := []string{"WETH", "USDC", "USDT", "USDT0"}
		if id == "privacy_pools" {
			expectedAssets = []string{"ETH"}
		}
		if len(assets) != len(expectedAssets) {
			return nil, "snapshot_unavailable"
		}
		for _, asset := range expectedAssets {
			balance := privacyObject(assets[asset])
			if balance == nil || !oneOf(privacyString(balance["status"]), "observed", "unknown") {
				return nil, "snapshot_unavailable"
			}
			for _, key := range []string{"amount", "usd"} {
				if balance["status"] == "unknown" {
					if balance[key] != nil {
						return nil, "snapshot_unavailable"
					}
				} else if !privacyMatch(`(0|[1-9][0-9]{0,35})(\.[0-9]{1,18})?`, balance[key]) {
					return nil, "snapshot_unavailable"
				}
			}
			if !fresh(balance["observed_at"]) {
				return nil, "stale_or_future_snapshot"
			}
		}
	}
	return p, ""
}

func privacyShieldedScreen(addresses []string, snapshot privacyMap, now time.Time) privacyMap {
	result := privacyRouteScreen(addresses, snapshot, now)
	clean := privacyMap{}
	for _, key := range []string{"status", "reason", "list_date", "list_version", "disclaimer"} {
		clean[key] = result[key]
	}
	return clean
}

func privacyShieldedDecimal(value *big.Rat) string {
	// Inputs have at most 18 decimal places; two basis-point deductions add 8.
	return strings.TrimRight(strings.TrimRight(value.FloatString(26), "0"), ".")
}

func privacyShieldedVenue(venue privacyMap, asset string, amount *big.Rat, now time.Time, sanctions privacyMap, bridge bool) (privacyMap, string) {
	poolAsset := asset
	if venue["id"] == "railgun" && oneOf(privacyString(venue["chain"]), "ethereum", "arbitrum") && asset == "ETH" {
		poolAsset = "WETH"
	}
	balance := privacyObject(privacyObject(venue["assets"])[poolAsset])
	if balance == nil {
		return nil, ""
	}
	addresses := []string{}
	for _, a := range privacyObject(venue["contracts"]) {
		addresses = append(addresses, privacyString(a))
	}
	screen := privacyShieldedScreen(addresses, sanctions, now)
	if screen["status"] != "not_listed" {
		return nil, "venue_sanctions_" + privacyString(screen["status"])
	}
	thinStatus, thinNote := "unknown", privacyShieldedNotice
	if balance["status"] == "observed" {
		pool, ok := new(big.Rat).SetString(privacyString(balance["amount"]))
		if !ok || pool.Sign() < 0 {
			return nil, "invalid_asset_balance"
		}
		thinStatus = "below_threshold"
		if new(big.Rat).Mul(amount, big.NewRat(100, 1)).Cmp(pool) >= 0 {
			thinStatus, thinNote = "warning", "thin crowd: amount is large relative to this asset balance"
		}
	}
	fee := privacyMap{"status": "unknown", "asset": poolAsset, "shield": nil, "unshield": nil, "total": nil, "received": nil, "formula": nil,
		"excluded": []any{"gas", "broadcaster", "bridge", "wrap"},
		"note":     "Protocol fee not established by the reviewed sources; no total route quote."}
	if venue["fee_bps"] != nil {
		shield := new(big.Rat).Mul(amount, big.NewRat(25, 10000))
		unshield := new(big.Rat).Mul(new(big.Rat).Sub(amount, shield), big.NewRat(25, 10000))
		total := new(big.Rat).Add(shield, unshield)
		fee["status"], fee["shield"], fee["unshield"] = "illustration", privacyShieldedDecimal(shield), privacyShieldedDecimal(unshield)
		fee["total"], fee["received"], fee["formula"] = privacyShieldedDecimal(total), privacyShieldedDecimal(new(big.Rat).Sub(amount, total)), privacyShieldedFormula
		fee["note"] = "Sequential 0.25% deductions, before gas and broadcaster costs; token-unit rounding is not modeled. Broadcaster premium is gas-based, not notional-based."
	}
	legs := []any{}
	if bridge {
		legs = append(legs, privacyMap{"leg": "bridge", "visibility": "public", "note": "Linkable public bridge; availability, asset mapping, cost and time are unverified."})
	}
	if poolAsset != asset {
		legs = append(legs, privacyMap{"leg": "wrap", "visibility": "public", "note": "ETH must become WETH before shielding; wrapping is public and its cost is unknown."})
	}
	legs = append(legs,
		privacyMap{"leg": "deposit", "visibility": "public", "note": "Funding wallet, asset, amount and time are public."},
		privacyMap{"leg": "shielded", "visibility": "conditional_private", "note": venue["private_leg"]},
		privacyMap{"leg": "exit", "visibility": "public", "note": venue["exit_note"]})
	return privacyMap{"venue": venue["id"], "chain": venue["chain"], "contracts": venue["contracts"], "asset": poolAsset,
		"fee": fee, "pool_size": balance, "thin_crowd": privacyMap{"status": thinStatus, "threshold": "amount >= 1% of reported asset balance", "note": thinNote},
		"compliance": venue["compliance"], "wait_note": venue["wait_note"], "legs": legs, "sanctions_screen": screen, "source_ids": venue["sources"]}, ""
}

func (n *privacyNative) shieldedRoute(input, snapshot, sanctions privacyMap) privacyMap {
	guide, _ := privacyTool("seconded_privacy_shielded_route")
	if !privacySchema(guide.InputSchema, input, 0) {
		return privacyRejected()
	}
	amount, ok := new(big.Rat).SetString(privacyString(input["amount"]))
	if !ok || amount.Sign() <= 0 {
		return privacyRejected()
	}
	now := n.now()
	report := privacyMap{"schema": "seconded-privacy-shielded-route/v1", "status": "unknown", "reason": nil, "charged": "no", "execution_allowed": false,
		"venues": []any{}, "nearest_options": []any{}, "funding_screen": privacyMap{"status": "not_provided"}, "snapshot": nil, "warnings": []any{privacyShieldedNotice}}
	if funding, exists := input["funding_wallet"]; exists {
		screen := privacyShieldedScreen([]string{privacyString(funding)}, sanctions, now)
		report["funding_screen"] = screen
		if screen["status"] != "not_listed" {
			if screen["status"] == "listed" {
				report["status"] = "blocked"
			}
			report["reason"] = "funding_sanctions_" + privacyString(screen["status"])
			return report
		}
	}
	p, reason := privacyShieldedSnapshot(snapshot, now)
	if p == nil {
		report["reason"] = reason
		return report
	}
	report["snapshot"] = privacyMap{"reviewed_at": p["reviewed_at"], "max_age_seconds": privacyShieldedMaxAge, "sources": p["sources"]}
	chain, asset := privacyString(input["chain"]), privacyString(input["asset"])
	noVenue := oneOf(chain, "base", "arc", "robinhood")
	if noVenue {
		report["reason"] = "no compliant shielded venue on this chain"
		report["warnings"] = append(privacyArray(report["warnings"]), "Nearest documented options require a public, linkable bridge; no bridge route is quoted or verified.")
	} else if privacyRailgunContracts[chain] == "" {
		report["reason"] = "unsupported_chain"
		return report
	}
	for _, value := range privacyArray(p["venues"]) {
		venue := privacyObject(value)
		if (noVenue && !oneOf(privacyString(venue["chain"]), "ethereum", "arbitrum")) || (!noVenue && venue["chain"] != chain) {
			continue
		}
		row, problem := privacyShieldedVenue(venue, asset, amount, now, sanctions, noVenue)
		if problem != "" {
			report["warnings"] = append(privacyArray(report["warnings"]), problem)
		}
		if row != nil {
			key := "venues"
			if noVenue {
				key = "nearest_options"
			}
			report[key] = append(privacyArray(report[key]), row)
		}
	}
	if !noVenue {
		if len(privacyArray(report["venues"])) > 0 {
			report["status"] = "compared"
		} else {
			report["reason"] = "no_reviewed_venue_for_asset_or_screening_unavailable"
		}
	}
	return report
}
