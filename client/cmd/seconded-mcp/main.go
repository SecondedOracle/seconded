package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	client "seconded.local/client"
)

var version = client.ClientVersion

const commandList = "Commands: setup, host-snippet, serve, self-check, prove, recover, limits, wallet, rollback, export-key, link coinbase, help, version"

var errMissingCommand = errors.New("missing_command: Specify a command.\n" + commandList + "\nRun seconded-mcp --help for usage.")

// Launch truth: payments default to Base mainnet; any other network, including a testnet, only when a check names it.
const (
	networkSummary = "Base, Arc and Robinhood Chain; Base mainnet by default, other networks only when a check names them"
	fundingNotice  = "Checks pay on Base mainnet unless they name another network: fund USDC on Base for them.\nOther networks only when a check names network=arc, robinhood, base_sepolia, arc_testnet or robinhood_testnet: USDC on Arc, USDG on Robinhood, testnet USDC or USDG on the testnets. Fund those only for the checks that name them."
)

func main() {
	defer func() {
		if recover() != nil {
			fmt.Fprintln(os.Stderr, "operation_failed")
			os.Exit(1)
		}
	}()
	if e := run(os.Args[1:]); e != nil {
		if errors.Is(e, errMissingCommand) {
			fmt.Fprintln(os.Stderr, e.Error())
			os.Exit(1)
		}
		if errors.Is(e, client.ErrTerminalPolicyRequired) {
			fmt.Fprintln(os.Stderr, e.Error())
			os.Exit(1)
		}
		if errors.Is(e, errExportTerminal) || errors.Is(e, errExportConfirmation) || errors.Is(e, errExportWrite) {
			fmt.Fprintln(os.Stderr, e.Error())
			os.Exit(1)
		}
		if errors.Is(e, client.ErrKeystoreData) {
			fmt.Fprintln(os.Stderr, client.ErrKeystoreData.Error()+": "+client.StorageErrorMessage(e))
			os.Exit(1)
		}
		if errors.Is(e, client.ErrStorage) {
			fmt.Fprintln(os.Stderr, "local_storage_unavailable: "+client.StorageErrorMessage(e))
			os.Exit(1)
		}
		switch e.Error() {
		case "release_digest_mismatch":
			fmt.Fprintln(os.Stderr, "release_digest_mismatch: This executable does not match the trusted release checksum. Stop and download the matching release again.")
		case "release_signature_unavailable":
			fmt.Fprintln(os.Stderr, "release_signature_unavailable: System OpenSSH ssh-keygen with working -Y verify support is required. Install or update system OpenSSH, then retry setup.")
		case "wallet_recovery_required":
			fmt.Fprintln(os.Stderr, "wallet_recovery_required: Wallet state exists without installation metadata. Preserve the profile and original executable; obtain owner-reviewed recovery.")
		case "wallet_already_exists":
			fmt.Fprintln(os.Stderr, "wallet_already_exists: No new policy was applied. Run setup again to display stored policy, or host-snippet --host HOST for host configuration.")
		case "release_signature_required", "release_signature_invalid", "release_signer_not_configured":
			fmt.Fprintln(os.Stderr, e.Error()+": Setup needs SHA-256SUMS and SHA-256SUMS.sig beside this executable, a compiled release signer, and system OpenSSH ssh-keygen. Obtain the complete signed release. For an independently verified development build, use --manual-fingerprint --release-sha256 DIGEST.")
		case "setup_advanced_required":
			fmt.Fprintln(os.Stderr, "setup_advanced_required: Custom limits, safety switches and file-key fallback require setup --advanced.")
		case "setup_required":
			fmt.Fprintln(os.Stderr, "setup_required: "+client.SetupRequiredMessage)
		case "keystore_required_use_allow_file_key", "release_digest_required", "fingerprint_mismatch", "release_identity_not_configured", "coinbase_link_not_implemented", "local_storage_unavailable", "profile_requires_absolute_path":
			fmt.Fprintln(os.Stderr, e.Error())
		default:
			fmt.Fprintln(os.Stderr, "operation_failed")
		}
		os.Exit(1)
	}
}
func run(args []string) error {
	profile := ""
	if len(args) > 0 && args[0] == "--profile" {
		if len(args) < 2 || !filepath.IsAbs(args[1]) {
			return errors.New("profile_requires_absolute_path")
		}
		profile = filepath.Clean(args[1])
		args = args[2:]
	}
	if len(args) == 0 {
		return errMissingCommand
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		fmt.Println("seconded-mcp " + version + " (" + networkSummary + ")\nUsage: seconded-mcp [--profile /absolute/directory] COMMAND [options]\n" + commandList)
		return nil
	}
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		fmt.Println("seconded-mcp " + version)
		return nil
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	exe, e = filepath.EvalSymlinks(exe)
	if e != nil {
		return e
	}
	dir := profile
	if dir == "" && args[0] != "host-snippet" {
		dir, e = client.DefaultDirectory()
		if e != nil {
			return e
		}
	}
	switch args[0] {
	case "wallet":
		f := flag.NewFlagSet("wallet", flag.ContinueOnError)
		f.SetOutput(io.Discard)
		human := f.Bool("human", false, "confirm on controlling terminal")
		target := f.String("switch", "", "back or newer")
		if f.Parse(args[1:]) != nil || f.NArg() != 0 || !*human || (*target != "back" && *target != "newer") {
			return client.ErrTerminalPolicyRequired
		}
		g, err := client.InstalledEngine(dir, exe)
		if err != nil {
			return err
		}
		result, err := g.TerminalSwitchWallet(*target == "back")
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	case "rollback":
		if len(args) != 1 {
			return client.ErrInvalid
		}
		return client.RollbackEnrollment(dir, exe)
	case "host-snippet":
		f := flag.NewFlagSet("host-snippet", flag.ContinueOnError)
		f.SetOutput(io.Discard)
		host := f.String("host", "generic", "")
		if f.Parse(args[1:]) != nil || f.NArg() != 0 {
			return client.ErrInvalid
		}
		snippet, err := client.HostSnippetProfile(*host, exe, profile)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, snippet)
		fmt.Fprintln(os.Stderr, client.HostInstructions(*host))
		return nil
	case "setup":
		f := flag.NewFlagSet("setup", flag.ContinueOnError)
		f.SetOutput(io.Discard)
		host := f.String("host", "generic", "")
		advanced := f.Bool("advanced", false, "set custom limits and switches without prompts")
		manual := f.Bool("manual-fingerprint", false, "use an independently verified release digest")
		digest := f.String("release-sha256", "", "independently verified SHA-256 for --manual-fingerprint")
		allow := f.Bool("allow-file-key", false, "")
		day := f.String("daily-limit", "25", "")
		hour := f.String("hourly-limit", "none", "")
		per := f.String("per-check-limit", "2.50", "")
		out := f.String("outstanding-limit", "none", "")
		gate := f.Bool("value-gate", false, "")
		dedupe := f.Bool("dedupe", true, "")
		loop := f.Bool("loop-brake", true, "")
		alerts := f.Bool("alerts", true, "")
		if f.Parse(args[1:]) != nil || f.NArg() != 0 {
			return client.ErrInvalid
		}
		if _, e = client.HostSnippet(*host, exe); e != nil {
			return e
		}
		settings := client.DefaultSettings()
		for _, p := range []struct {
			s   string
			dst **int64
		}{{*day, &settings.Limits.Day}, {*hour, &settings.Limits.Hour}, {*per, &settings.Limits.PerCheck}, {*out, &settings.Limits.Outstanding}} {
			if p.s == "none" || p.s == "no_limit" {
				*p.dst = nil
			} else {
				v, e := client.USD(p.s)
				if e != nil {
					return e
				}
				*p.dst = &v
			}
		}
		settings.ValueGate = *gate
		settings.Dedupe = *dedupe
		settings.LoopBrake = *loop
		settings.Alerts = *alerts
		// Unspecified limits retain safe defaults.
		chosen := 0
		custom := false
		f.Visit(func(v *flag.Flag) {
			switch v.Name {
			case "daily-limit", "hourly-limit", "per-check-limit", "outstanding-limit", "value-gate", "dedupe", "loop-brake", "alerts", "allow-file-key":
				custom = true
			}
			if v.Name == "daily-limit" || v.Name == "hourly-limit" || v.Name == "per-check-limit" || v.Name == "outstanding-limit" {
				chosen++
			}
		})
		if custom && !*advanced {
			return errors.New("setup_advanced_required")
		}
		if (*manual && *digest == "") || (!*manual && *digest != "") {
			return errors.New("release_digest_required")
		}
		install, e := client.RunSetupWithOptions(strings.NewReader(*digest), os.Stderr, nil, dir, client.ProfileOSStore(dir), exe, settings, client.SetupOptions{Advanced: *advanced, ManualFingerprint: *manual, LimitsChosen: chosen == 4, AllowFile: *allow})
		if e != nil {
			if command := client.ProfilePermissionCommand(dir, e); command != "" {
				fmt.Fprintln(os.Stderr, "Make the profile directory private, then retry setup: "+command)
			}
			return e
		}
		if e = client.PrintSetupProfile(os.Stdout, install, *host, profile); e != nil {
			return e
		}
		fmt.Fprintln(os.Stderr, fundingNotice+"\n"+client.HostInstructions(*host)+"\nPreserve existing settings; use user-level settings only.\nNo private key environment variable is needed. No automatic updates.")
		if install.Backend == "file" {
			fmt.Fprintln(os.Stderr, "File-key mode: wallet.key is protected by filesystem permissions only.")
		}
		return nil
	case "serve":
		f := flag.NewFlagSet("serve", flag.ContinueOnError)
		f.SetOutput(io.Discard)
		host := f.String("host", "generic", "")
		originalDoor := f.Bool("original-door-rollback", false, "explicit original payment-door rollback; never automatically selected")
		if f.Parse(args[1:]) != nil || f.NArg() != 0 {
			return client.ErrInvalid
		}
		if _, e = client.HostSnippet(*host, exe); e != nil {
			return e
		}
		g, e := client.InstalledEngine(dir, exe)
		if e != nil && !errors.Is(e, client.ErrSetupRequired) {
			return e
		}
		if g != nil && *originalDoor {
			g.API.UseOriginalDoorRollback()
		}
		return client.ServeWithSetup(context.Background(), os.Stdin, os.Stdout, g, func() (*client.Engine, client.Installation, error) {
			install, err := client.RunSetupWithOptions(strings.NewReader(""), io.Discard, nil, dir, client.ProfileOSStore(dir), exe, client.QuickSettings(), client.SetupOptions{})
			if err != nil {
				return nil, install, err
			}
			engine, err := client.InstalledEngine(dir, exe)
			if err == nil && *originalDoor {
				engine.API.UseOriginalDoorRollback()
			}
			return engine, install, err
		})
	case "recover":
		f := flag.NewFlagSet("recover", flag.ContinueOnError)
		f.SetOutput(io.Discard)
		id := f.String("check-id", "", "local recovery/check ID; omit to list pending purchases")
		if f.Parse(args[1:]) != nil || f.NArg() != 0 {
			return client.ErrInvalid
		}
		files, err := client.OpenFiles(dir)
		if err != nil {
			return err
		}
		api, err := client.ReleaseAPI()
		if err != nil {
			return err
		}
		// No wallet or signing key is loaded by standard-door recovery.
		g := &client.Engine{Files: files, API: api}
		result, err := g.Receipts(context.Background(), *id)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"checks": result})
	case "prove":
		if len(args) != 3 {
			return client.ErrInvalid
		}
		files, err := client.OpenFiles(dir)
		if err != nil {
			return err
		}
		input, err := os.Open(args[2])
		if err != nil {
			return err
		}
		defer input.Close()
		raw, err := io.ReadAll(io.LimitReader(input, client.MessageLimit+1))
		if err != nil {
			return err
		}
		var request client.Request
		if client.DecodeStrict(raw, &request, client.MessageLimit) != nil {
			return client.ErrInvalid
		}
		if err = client.ProveLocalReceipt(files, args[1], request); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"check_id": args[1], "input_binding_verified": true})
	case "self-check":
		if len(args) != 1 {
			return client.ErrInvalid
		}
		g, e := client.InstalledEngine(dir, exe)
		if e != nil {
			return e
		}
		v, e := client.LoadVault(g.Store)
		if e != nil {
			return e
		}
		h, e := client.Fingerprint(exe)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"version": version, "sha256": h, "address": v.Address, "fingerprint_matches": true, "testnet_only": false, "default_network": client.DefaultPaymentNetwork, "live_api_configured": g.API != nil})
	case "link":
		if len(args) != 2 || args[1] != "coinbase" {
			return client.ErrInvalid
		}
		_, e := (client.CoinbaseStub{}).Link()
		return e
	case "export-key":
		// No destination, input flag or environment override can bypass the terminal.
		if len(args) != 1 {
			return client.ErrInvalid
		}
		return exportKey(dir, exe)
	case "limits":
		f := flag.NewFlagSet("limits", flag.ContinueOnError)
		f.SetOutput(io.Discard)
		show := f.Bool("show", false, "show stored policy")
		set := f.String("set", "", "bounded policy change: name=value")
		patch := f.String("patch", "", "typed policy JSON, using MCP limit field names")
		human := f.Bool("human", false, "confirm exact policy change on the controlling terminal")
		if f.Parse(args[1:]) != nil || f.NArg() != 0 {
			return client.ErrInvalid
		}
		var hasSet, hasPatch bool
		f.Visit(func(v *flag.Flag) {
			hasSet = hasSet || v.Name == "set"
			hasPatch = hasPatch || v.Name == "patch"
		})
		if hasSet && hasPatch || hasSet && *set == "" || hasPatch && *patch == "" || *show && (hasSet || hasPatch || *human) || *human && !hasSet && !hasPatch {
			return client.ErrInvalid
		}
		g, err := client.InstalledEngine(dir, exe)
		if err != nil {
			return err
		}
		if !hasSet && !hasPatch {
			result, err := g.Limits(json.RawMessage(`{}`), false)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(result)
		}
		if hasSet {
			parts := strings.Split(*set, "=")
			if len(parts) != 2 {
				return client.ErrInvalid
			}
			fields := map[string]any{}
			if name, ok := map[string]string{"day": "daily_usd", "hour": "hourly_usd", "per_check": "per_check_usd", "outstanding": "outstanding_usd"}[parts[0]]; ok {
				fields[name] = parts[1]
			} else {
				switch parts[0] {
				case "value_gate", "dedupe", "loop_brake", "alerts", "frozen":
					if parts[1] != "on" && parts[1] != "off" {
						return client.ErrInvalid
					}
					fields[parts[0]] = parts[1] == "on"
				default:
					return client.ErrInvalid
				}
			}
			raw, err := json.Marshal(fields)
			if err != nil {
				return err
			}
			*patch = string(raw)
		}
		var result any
		if *human {
			// Engine APIs manage their own locks and release them while the owner types.
			var proof *client.TerminalPolicyProof
			proof, err = g.ConfirmTerminalPolicy(json.RawMessage(*patch))
			if err != nil {
				return err
			}
			result, err = g.TerminalLimits(json.RawMessage(*patch), proof)
		} else {
			result, err = g.Limits(json.RawMessage(*patch), true)
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"limits": result})
	default:
		return errors.New("unknown_command")
	}
}
