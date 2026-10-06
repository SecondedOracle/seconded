package client

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type limitField struct {
	name string
	old  *int64
	new  *int64
}

func limitFields(a, b Settings) []limitField {
	return []limitField{{"per-check", a.Limits.PerCheck, b.Limits.PerCheck}, {"hourly", a.Limits.Hour, b.Limits.Hour}, {"daily", a.Limits.Day, b.Limits.Day}, {"outstanding", a.Limits.Outstanding, b.Limits.Outstanding}}
}

type switchField struct {
	name     string
	old, new bool
}

func switchFields(a, b Settings) []switchField {
	return []switchField{{"value gate", a.ValueGate, b.ValueGate}, {"dedupe", a.Dedupe, b.Dedupe}, {"loop brake", a.LoopBrake, b.LoopBrake}, {"alerts", a.Alerts, b.Alerts}}
}

// DescribeChanges lists policy differences for callers displaying a summary.
// The summary does not trigger confirmation or authentication.
func DescribeChanges(current, next Settings) []string {
	out := []string{}
	for _, l := range limitFields(current, next) {
		switch {
		case l.old == nil:
		case l.new == nil:
			out = append(out, fmt.Sprintf("Remove %s limit (currently $%s)", l.name, Dollars(*l.old)))
		case *l.new > *l.old:
			out = append(out, fmt.Sprintf("Raise %s limit from $%s to $%s", l.name, Dollars(*l.old), Dollars(*l.new)))
		}
	}
	if current.Frozen && !next.Frozen {
		out = append(out, "Unfreeze new payment authorizations (currently frozen)")
	}
	for _, s := range switchFields(current, next) {
		if s.old && !s.new {
			out = append(out, fmt.Sprintf("Turn off %s (currently on)", s.name))
		}
	}
	return out
}

// DescribeSetup describes departures from safe defaults without gating setup.
func DescribeSetup(s Settings, allowFile bool) []string {
	out := DescribeChanges(QuickSettings(), s)
	if allowFile {
		out = append(out, "Allow the wallet private key in a plain file (wallet.key) if the OS key store is unavailable")
	}
	return out
}

func PrintPolicy(w io.Writer, s Settings, allowFile bool) error {
	var b strings.Builder
	b.WriteString("SECONDED setup policy (Base, Arc and Robinhood; Base mainnet default, other networks only when named):\n")
	for _, l := range limitFields(s, s) {
		v := "none"
		if l.new != nil {
			v = "$" + Dollars(*l.new)
		}
		fmt.Fprintf(&b, "  %s limit: %s\n", l.name, v)
	}
	for _, f := range switchFields(s, s) {
		v := "off"
		if f.new {
			v = "on"
		}
		fmt.Fprintf(&b, "  %s: %s\n", f.name, v)
	}
	fileKey := "automatic when the OS credential store is unavailable"
	if allowFile {
		fileKey = "allowed"
	}
	fmt.Fprintf(&b, "  file-key fallback: %s\n  compiled maximum per authorization: $%s (always enforced)\n", fileKey, Dollars(MaxAuthorization))
	fmt.Fprintf(&b, "  new authorizations frozen: %t\n", s.Frozen)
	_, e := io.WriteString(w, b.String())
	return e
}

// VerifyReleaseDigest accepts an explicit digest from the caller without prompting.
func VerifyReleaseDigest(in io.Reader, _ io.Writer, exe string) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(in, 66))
	want := strings.ToLower(strings.TrimSpace(string(raw)))
	if err != nil || !hexDigest.MatchString(want) {
		return "", errors.New("release_digest_required")
	}
	got, err := Fingerprint(exe)
	if err != nil {
		return "", err
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return "", errors.New("release_digest_mismatch")
	}
	return want, nil
}

// RunSetup preserves the advanced, manual-fingerprint library entry point.
// New CLI setup uses RunSetupWithOptions with safe quick defaults.
func RunSetup(in io.Reader, out io.Writer, owner OwnerConfirmation, dir string, store SecretStore, exe string, settings Settings, limitsChosen, allowFile bool) (Installation, error) {
	return RunSetupWithOptions(in, out, owner, dir, store, exe, settings, SetupOptions{
		Advanced: true, ManualFingerprint: true, LimitsChosen: limitsChosen, AllowFile: allowFile,
	})
}

type SetupOptions struct {
	Advanced          bool
	ManualFingerprint bool
	LimitsChosen      bool
	AllowFile         bool
}

func RunSetupWithOptions(in io.Reader, out io.Writer, _ OwnerConfirmation, dir string, store SecretStore, exe string, settings Settings, options SetupOptions) (result Installation, err error) {
	defer func() { err = setupStorageError(err, dir) }()
	if !filepath.IsAbs(dir) {
		return Installation{}, ErrStorage
	}
	// A setup rerun displays stored state; settings changes use the limits tools.
	if _, err := os.Lstat(filepath.Join(dir, "install.json")); err == nil {
		files := &Files{Dir: dir}
		b, err := files.Read("install.json")
		var install Installation
		if err != nil || DecodeStrict(b, &install, ResponseLimit) != nil {
			return Installation{}, ErrStorage
		}
		if install.Backend != "file" && install.Backend != "os_keystore" {
			return Installation{}, ErrStorage
		}
		store = activeWalletStore{files, store}
		if err = CheckFingerprint(files, store, exe); err != nil {
			return Installation{}, err
		}
		engine := &Engine{Files: files, Store: activeWalletStore{files, store}}
		if selected, ok := store.(activeWalletStore); ok {
			engine.Store = selected
		}
		if err := engine.discoverEarlierWallet(context.Background()); err != nil {
			return Installation{}, err
		}
		install, err = readInstallation(files)
		if err != nil {
			return Installation{}, err
		}
		vault, err := LoadVault(store)
		if err != nil {
			return Installation{}, err
		}
		if _, err = fmt.Fprintln(out, "Existing wallet: stored policy below; no policy changes applied. Use limits to change policy, or host-snippet for another host."); err != nil {
			return Installation{}, err
		}
		if install.Backend == "file" {
			if _, err = fmt.Fprintln(out, WalletExistingMessage(install, dir)); err != nil {
				return Installation{}, err
			}
		}
		return install, PrintPolicy(out, vault.Settings, install.Backend == "file")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Installation{}, ErrStorage
	}
	if !options.Advanced {
		if options.AllowFile || options.LimitsChosen {
			return Installation{}, errors.New("setup_advanced_required")
		}
		settings = QuickSettings()
	}
	var digest string
	var e error
	if options.ManualFingerprint {
		digest, e = VerifyReleaseDigest(in, out, exe)
	} else {
		digest, e = VerifyAutomaticReleaseDigest(exe)
	}
	if e != nil {
		return Installation{}, e
	}
	// Missing install metadata is an orphaned wallet, not permission to replace
	// its key. Detect it before displaying a policy that cannot be installed.
	if _, err := store.Get("vault"); err == nil {
		return Installation{}, errors.New("wallet_recovery_required")
	}
	for _, name := range []string{"wallet.key", "ledger.json"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			return Installation{}, errors.New("wallet_recovery_required")
		} else if !errors.Is(err, os.ErrNotExist) {
			return Installation{}, ErrStorage
		}
	}
	if e = settings.Validate(); e != nil {
		return Installation{}, e
	}
	if e = ChatPolicy(QuickSettings(), settings); e != nil {
		return Installation{}, ErrTerminalPolicyRequired
	}
	if e = PrintPolicy(out, settings, options.AllowFile); e != nil {
		return Installation{}, e
	}
	files, e := OpenFiles(dir)
	if e != nil {
		return Installation{}, e
	}
	install, err := SetupWallet(files, store, options.AllowFile, settings, exe, digest)
	if err != nil {
		return install, err
	}
	_, err = fmt.Fprintln(out, WalletFundingMessage(install, dir))
	return install, err
}

// TerminalPolicyProof has no public fields or deserialization path. Only
// ConfirmTerminalPolicy mints it, after reading the controlling terminal.
// Copies share consumption state, so copying the object cannot replay approval.
type TerminalPolicyProof struct{ state *terminalPolicyState }
type terminalPolicyState struct {
	mu      sync.Mutex
	binding [32]byte
	expires time.Time
	used    bool
}

func policyBinding(profile string, v Vault, next Settings) [32]byte {
	raw, _ := json.Marshal(struct {
		Profile, Address, Fingerprint, Revision string
		Before, After                           Settings
	}{filepath.Clean(profile), v.Address, v.Fingerprint, v.PolicyRevision, v.Settings, next})
	return sha256.Sum256(raw)
}

// Include selection revision even if the owner switches away and back while
// confirmation is pending.
func (g *Engine) terminalBinding(v Vault, next Settings) ([32]byte, error) {
	i, err := readInstallation(g.Files)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return [32]byte{}, err
	}
	base := policyBinding(g.Files.Dir, v, next)
	return sha256.Sum256(append(base[:], []byte(i.PolicyRevision)...)), nil
}

// ConfirmTerminalPolicy is for the human CLI only; MCP never calls it. It opens
// /dev/tty itself rather than accepting stdin, a supplied answer, or a callback.
// Windows uses its console handles; ordinary pipes cannot supply approval.
func (g *Engine) ConfirmTerminalPolicy(args json.RawMessage) (*TerminalPolicyProof, error) {
	var patch limitUpdate
	if err := DecodeStrict(args, &patch, MessageLimit); err != nil {
		return nil, err
	}
	unlock, err := g.Files.Lock()
	if err != nil {
		return nil, err
	}
	v, err := LoadVault(g.Store)
	if err != nil {
		unlock()
		return nil, err
	}
	next, err := patchSettings(v.Settings, patch)
	if err != nil {
		unlock()
		return nil, err
	}
	binding, err := g.terminalBinding(v, next)
	unlock()
	if err != nil {
		return nil, err
	}
	state := &terminalPolicyState{binding: binding, expires: time.Now().Add(2 * time.Minute)}
	terminal := "/dev/tty"
	if runtime.GOOS == "windows" {
		terminal = "CONIN$"
	}
	tty, err := os.OpenFile(terminal, os.O_RDWR, 0)
	if err != nil {
		return nil, g.terminalPolicyError(patch)
	}
	defer tty.Close()
	info, err := tty.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return nil, g.terminalPolicyError(patch)
	}
	output := tty
	if runtime.GOOS == "windows" {
		output, err = os.OpenFile("CONOUT$", os.O_WRONLY, 0)
		if err != nil {
			return nil, g.terminalPolicyError(patch)
		}
		defer output.Close()
	}
	change := newPolicyChange(v.Settings, next, v.Address, v.Address, "human terminal")
	if _, err = fmt.Fprintf(output, "%s\nWallet: %s\nType the last 6 address characters to confirm: ", policyDescription(change), v.Address); err != nil {
		return nil, ErrStorage
	}
	answer, err := bufio.NewReader(io.LimitReader(tty, 64)).ReadString('\n')
	if err != nil || !strings.EqualFold(strings.TrimSuffix(strings.TrimSuffix(answer, "\n"), "\r"), v.Address[len(v.Address)-6:]) || time.Now().After(state.expires) {
		return nil, g.terminalPolicyError(patch)
	}
	if err = ConfirmOwnerPresence(policyDescription(change)+" Wallet: "+v.Address+" Profile: "+g.Files.Dir, tty, output); err != nil || time.Now().After(state.expires) {
		return nil, g.terminalPolicyError(patch)
	}
	return &TerminalPolicyProof{state: state}, nil
}

// TerminalLimits consumes exact-change approval while holding the same profile
// lock as checks, chat patches, freezes and wallet selection.
func (g *Engine) TerminalLimits(args json.RawMessage, proof *TerminalPolicyProof) (any, error) {
	var patch limitUpdate
	if err := DecodeStrict(args, &patch, MessageLimit); err != nil {
		return nil, err
	}
	unlock, err := g.Files.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	v, err := LoadVault(g.Store)
	if err != nil {
		return nil, err
	}
	next, err := patchSettings(v.Settings, patch)
	if err != nil {
		return nil, err
	}
	if proof == nil || proof.state == nil {
		return nil, g.terminalPolicyError(patch)
	}
	state := proof.state
	state.mu.Lock()
	defer state.mu.Unlock()
	binding, err := g.terminalBinding(v, next)
	if err != nil {
		return nil, err
	}
	if state.used || time.Now().After(state.expires) || state.binding != binding {
		return nil, g.terminalPolicyError(patch)
	}
	state.used = true
	if err = g.savePolicyLocked(v, next, "human terminal"); err != nil {
		return nil, err
	}
	return publicLimits(next), nil
}
