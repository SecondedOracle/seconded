//go:build !staging

package client

import (
	"crypto/ed25519"
	_ "embed"
)

//go:embed products.json
var deploymentProductGuideJSON []byte

const DefaultPaymentNetwork = "eip155:8453"

func deploymentPaymentNetwork(network string) bool { return true }

func apiOrigin() string { return releaseAPIOrigin }

func receiptKeysFor(network string) map[string]ed25519.PublicKey {
	return productionReceiptKeysFor(network)
}

func toolAvailability(status string) string { return status }
func describeTools(tools []Tool) []Tool     { return tools }
func deploymentProductTiers(known map[string]int) map[string]int {
	for _, product := range []string{"job_escrow_check", "x402_payment_check", "shielded_route_check", "route_check"} {
		known[product] = 1
	}
	return known
}
func deploymentReceiptLabels(labels map[string]map[int]string) map[string]map[int]string {
	for _, product := range []string{"job_escrow_check", "x402_payment_check", "shielded_route_check", "route_check"} {
		labels[product] = map[int]string{1: "proceed", 2: "do_not_proceed", 3: "not_verified"}
	}
	return labels
}
