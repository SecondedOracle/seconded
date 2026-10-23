package client
import ("os"; "strings"; "testing")
func TestPublicCreditBalanceCompatibility(t *testing.T) {
 base, err := os.ReadFile("testdata/credit-balance-legacy.json")
 if err != nil { t.Fatal(err) }
 prefix := strings.TrimSuffix(strings.TrimSpace(string(base)), "}")
 for _, fields := range []string{"", `,"credit_balance_usd_micros":0,"credit_balance_basis":"after_reservation","credit_balance_observed_at":1800000000`} {
  r, err := parseReply(200, []byte(prefix+fields+"}"))
  if err != nil { t.Fatal(err) }
  if (r.CreditBalanceMicros == nil) != (fields == "") { t.Fatal("missing snapshot became zero") }
 }
 if _, err := parseReply(200, []byte(prefix+`,"unexpected":1}`)); err == nil { t.Fatal("unknown field accepted") }
}
