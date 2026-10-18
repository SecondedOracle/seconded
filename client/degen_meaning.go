package client

import "time"

// Additive meaning fields remain optional for historical signed answers.
func (a DegenAnswer) validMeaning() bool {
	if a.OutcomeCause != "" && !oneOf(a.OutcomeCause, "insufficient_facts", "judge_unavailable_or_invalid", "models_differ", "judges_agree", "budget_paused") {
		return false
	}
	if a.VerdictAt != "" {
		stamp, err := time.Parse(time.RFC3339, a.VerdictAt)
		if err != nil {
			return false
		}
		if a.VerdictExpiresAt != nil {
			expires, err := time.Parse(time.RFC3339, *a.VerdictExpiresAt)
			if err != nil || !expires.Equal(stamp.Add(30*time.Minute)) {
				return false
			}
		}
	} else if a.VerdictExpiresAt != nil {
		return false
	}
	if s := a.JudgedSnapshot; s != nil {
		if !configFingerprint.MatchString(s.EvidenceSHA256) || len(s.Facts) > 32 {
			return false
		}
		for id, f := range s.Facts {
			if id != f.ID || !oneOf(id, degenFactIDs...) || !oneOf(f.Status, "observed", "not_available") || (f.Block != nil && (*f.Block < 0 || *f.Block > maxSafeInteger)) {
				return false
			}
		}
	}
	for _, m := range a.Models {
		if m.VerdictAt != "" {
			if _, err := time.Parse(time.RFC3339, m.VerdictAt); err != nil || m.VerdictAt != a.VerdictAt {
				return false
			}
		}
		for _, r := range m.Reasons {
			if r.EvidenceScope != "" && r.EvidenceScope != "judged_snapshot" {
				return false
			}
			if r.EvidenceScope != "" {
				if a.JudgedSnapshot == nil {
					return false
				}
				for _, id := range r.EvidenceIDs {
					if _, ok := a.JudgedSnapshot.Facts[id]; !ok {
						return false
					}
				}
			}
		}
	}
	for _, row := range a.Scorecard {
		if row.Block != nil && (*row.Block < 0 || *row.Block > maxSafeInteger) {
			return false
		}
		if row.ObservedAt != nil {
			if _, err := time.Parse(time.RFC3339, *row.ObservedAt); err != nil {
				return false
			}
		}
		if row.MeasurementKind != "" && !oneOf(row.MeasurementKind, "full_route", "sell_quote", "curve_simulation", "virtual_reserve", "actual_reserve") {
			return false
		}
		if row.Band != "" && !oneOf(row.Band, "na", "low", "moderate", "high") {
			return false
		}
		if row.Coverage != "" && !oneOf(row.Coverage, "complete", "partial", "unknown") {
			return false
		}
	}
	if a.Holder.Coverage != "" && !oneOf(a.Holder.Coverage, "complete", "partial", "unknown") {
		return false
	}
	if a.PossibleBundle != nil && a.PossibleBundle.MeasurementKind != "" && a.PossibleBundle.MeasurementKind != "same_block_recipients" {
		return false
	}
	if m := a.Market; m != nil {
		if m.PriceBasis != "" && !oneOf(m.PriceBasis, degenPriceBasis...) {
			return false
		}
		if m.CapBasis != "" && !oneOf(m.CapBasis, degenCapBasis...) {
			return false
		}
		if m.SupplyAsOf != nil {
			stamp, err := time.Parse(time.RFC3339, *m.SupplyAsOf)
			at, _ := time.Parse(time.RFC3339, a.AsOf.Time)
			if err != nil || stamp.After(at) {
				return false
			}
		}
	}
	return true
}
