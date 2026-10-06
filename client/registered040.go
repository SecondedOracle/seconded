package client

// Product exposure is selected by the deployment profile; runtime settings
// cannot add products to the release catalog.
var registered040Products = []string{
	"x402_payment_check", "vault_check", "shielded_route_check", "private_receive_scan", "route_check",
}
