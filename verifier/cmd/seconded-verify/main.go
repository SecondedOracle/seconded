// Command seconded-verify checks SECONDED receipts offline.
//
//	seconded-verify [-keys keys.json] [-json] receipt.json [more.json ...]
//
// Exit status 0 when every receipt verifies, 1 otherwise. It makes no network
// calls and reads nothing but the files named on the command line.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/SecondedOracle/seconded/verifier"
)

func main() {
	keysPath := flag.String("keys", "", "path to a /v1/keys document to use instead of the compiled pins")
	asJSON := flag.Bool("json", false, "print one JSON object per receipt")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: seconded-verify [-keys keys.json] [-json] receipt.json [more.json ...]")
		fmt.Fprintln(os.Stderr, "Pinned receipt keys:")
		for id, hexKey := range verifier.ReleaseKeysHex {
			fmt.Fprintf(os.Stderr, "  %s %s\n", id, hexKey)
		}
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	keys := verifier.ReleaseKeys()
	if *keysPath != "" {
		data, err := os.ReadFile(*keysPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "keys:", err)
			os.Exit(2)
		}
		keys, err = verifier.KeysFromDocument(data)
		if err != nil {
			fmt.Fprintln(os.Stderr, "keys:", err)
			os.Exit(2)
		}
	}
	failed := false
	for _, path := range flag.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			failed = true
			report(*asJSON, path, nil, err)
			continue
		}
		result, err := verifier.Verify(data, keys)
		if err != nil {
			failed = true
		}
		report(*asJSON, path, result, err)
	}
	if failed {
		os.Exit(1)
	}
}

func str(p *string) string {
	if p == nil {
		return "null"
	}
	return *p
}

func report(asJSON bool, path string, r *verifier.Report, err error) {
	if asJSON {
		out := map[string]any{"file": path, "verified": err == nil}
		if err != nil {
			out["error"] = err.Error()
		} else {
			e := r.Envelope
			out["key_id"] = r.KeyID
			out["version"] = r.Version
			out["state"] = e.State
			out["state_meaning"] = verifier.States[e.State]
			out["outcome"] = e.Outcome
			out["product"] = e.Product
			out["check_id"] = e.CheckID
			out["issued_at"] = e.IssuedAt
			out["checked_by"] = e.CheckedBy
			out["billing"] = e.Billing
			if e.Answer != nil {
				out["label_id"] = e.Answer.LabelID
			}
			if e.Status != "" {
				out["status"] = e.Status
				out["message"] = e.Message
				out["reason"] = e.Reason
			}
		}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}
	if err != nil {
		fmt.Printf("FAIL  %s\n      %v\n", path, err)
		return
	}
	e := r.Envelope
	labs := "none named"
	if len(e.CheckedBy) > 0 {
		labs = strings.Join(e.CheckedBy, " + ")
	}
	fmt.Printf("OK    %s\n", path)
	fmt.Printf("      signature   Ed25519 by %s over receipt v%d (%d canonical bytes)\n", r.KeyID, r.Version, len(r.Canonical))
	fmt.Printf("      state       %s: %s\n", e.State, verifier.States[e.State])
	fmt.Printf("      product     %s   check_id %s   issued_at %s\n", str(e.Product), str(e.CheckID), e.IssuedAt)
	fmt.Printf("      checked_by  %s\n", labs)
	if e.Answer != nil {
		fmt.Printf("      answer      label %s (option %d)\n", e.Answer.LabelID, e.Answer.Option)
	}
	if e.Status != "" {
		fmt.Printf("      status      %s (%s): %s\n", e.Status, e.Reason, e.Message)
	}
	fmt.Printf("      billing     charged=%s settlement=%s network=%s amount_atomic=%s tx=%s\n", e.Billing.Charged, e.Billing.Settlement, e.Billing.Network, str(e.Billing.Amount), str(e.Billing.Tx))
}
