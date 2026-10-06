package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

var ErrSetupRequired = errors.New("setup_required")

const SetupRequiredMessage = "SECONDED is not set up yet: run `seconded-mcp setup` with this executable and the same --profile, if configured. Safe default setup is non-interactive; the MCP server can also create the wallet on the first tool call and return its funding address."

// Only recommend chmod when the existing directory actually has loose Unix
// permissions. Corrupt credentials, symlinks and other storage failures need
// their existing guidance instead.
func ProfilePermissionCommand(dir string, err error) string {
	if runtime.GOOS == "windows" || !errors.Is(err, ErrStorage) || !filepath.IsAbs(dir) {
		return ""
	}
	info, statErr := os.Lstat(dir)
	if statErr != nil || !info.IsDir() || info.Mode().Perm()&0077 == 0 {
		return ""
	}
	return "chmod 700 '" + strings.ReplaceAll(dir, "'", "'\"'\"'") + "'"
}

func HostSnippet(host, exe string) (string, error) {
	return HostSnippetProfile(host, exe, "")
}
func HostSnippetProfile(host, exe, profile string) (string, error) {
	args := []string{"serve", "--host", host}
	if profile != "" {
		if !filepath.IsAbs(profile) {
			return "", ErrInvalid
		}
		args = append([]string{"--profile", profile}, args...)
	}
	if !filepath.IsAbs(exe) {
		return "", ErrInvalid
	}
	switch host {
	case "claude-code":
		words := []string{"claude", "mcp", "add", "--scope", "user", "seconded", "--", exe}
		words = append(words, args...)
		for i, word := range words {
			if i < 7 {
				continue
			}
			if runtime.GOOS == "windows" {
				words[i] = "'" + strings.ReplaceAll(word, "'", "''") + "'"
			} else {
				words[i] = "'" + strings.ReplaceAll(word, "'", "'\"'\"'") + "'"
			}
		}
		prefix := ""
		if runtime.GOOS == "windows" {
			prefix = "& " // PowerShell call operator for a quoted executable.
		}
		return prefix + strings.Join(words, " ") + "\n", nil
	case "generic", "cursor", "claude-desktop", "gemini":
		b, e := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"seconded": map[string]any{"command": exe, "args": args}}}, "", "  ")
		return string(b), e
	case "codex":
		encoded, err := json.Marshal(args)
		return "[mcp_servers.seconded]\ncommand = " + strconv.Quote(exe) + "\nargs = " + string(encoded) + "\n", err
	default:
		return "", ErrInvalid
	}
}

// ChooseLimits preserves the caller's explicit settings without terminal input.
func ChooseLimits(_ io.Reader, _ io.Writer, settings Settings) (Settings, error) {
	return settings, settings.Validate()
}

// WalletCreatedMessage is safe for both the setup terminal and MCP output.
func WalletCreatedMessage(address string) string {
	return "SECONDED created a check wallet: " + address + ". Fund it with a few dollars of USDC on Base or Arc, or USDG on Robinhood Chain to use SECONDED checks. SECONDED requires a dedicated funded wallet for security: keep only what checks need in it. New setup defaults: $2.50 max per check, $25 a day; ask your agent only to tighten or freeze. Raising or removing limits, weakening safety switches, unfreezing and switching wallets require a human terminal. On upgrade, legacy profiles without a daily cap or above $25/day are frozen pending owner review."
}

func WalletFundingMessage(install Installation, dir string) string {
	message := WalletCreatedMessage(install.Address) + " Storage backend: " + install.Backend + "."
	if install.Backend == "file" {
		message += " Wallet key stored in " + filepath.Join(dir, walletFilename(install)) + ", private to your user account; back it up. This file is protected by filesystem permissions, not encrypted by SECONDED."
	}
	if install.EarlierCheckPending {
		message += "\n" + EarlierWalletWarning
	}
	return message
}

const EarlierWalletWarning = "Couldn't check for an earlier SECONDED wallet on this computer. If you had one, don't fund this address yet - ask me to check again once your computer is unlocked."

func WalletExistingMessage(install Installation, dir string) string {
	return strings.Replace(WalletFundingMessage(install, dir), "SECONDED created a check wallet:", "SECONDED check wallet:", 1)
}

func InstalledEngine(dir, exe string) (*Engine, error) {
	files, e := OpenFiles(dir)
	if e != nil {
		return nil, e
	}
	b, e := files.Read("install.json")
	if errors.Is(e, ErrNotFound) {
		return nil, ErrSetupRequired
	}
	if e != nil {
		return nil, e
	}
	var i Installation
	if DecodeStrict(b, &i, ResponseLimit) != nil {
		return nil, ErrStorage
	}
	var store SecretStore = ProfileOSStore(dir)
	if i.Backend == "file" {
		store = activeWalletStore{files, ProfileOSStore(dir)}
	} else if i.Backend != "os_keystore" {
		return nil, ErrStorage
	}
	if e = CheckFingerprint(files, store, exe); e != nil {
		return nil, e
	}
	unlock, e := files.Lock()
	if e != nil {
		return nil, e
	}
	l, e := ReadLedger(files)
	if e == nil {
		for j := range l.Entries {
			if l.Entries[j].State == "RESERVED" {
				l.Entries[j].State = "RELEASED"
				l.Entries[j].PaymentDoor = "" // No credential escaped an unsigned reservation.
			}
		}
		e = l.Save(files)
	}
	unlock()
	if e != nil {
		return nil, e
	}
	api, e := ReleaseAPI()
	if e != nil && e.Error() != "release_identity_not_configured" {
		return nil, e
	}
	// Without a configured quorum there is no chain evidence at all: checks are
	// budget-blocked and nothing is ever reported as not charged.
	var chain ChainReader
	quorum, e := ReleaseEvidence()
	if e == nil {
		chain = releaseChainReader{quorum}
	} else if !errors.Is(e, errEvidenceNotConfigured) {
		return nil, e
	}
	engine := &Engine{files, activeWalletStore{files, ProfileOSStore(dir)}, api, chain}
	if err := engine.discoverEarlierWallet(context.Background()); err != nil {
		return nil, err
	}
	return engine, nil
}

func PrintSetup(w io.Writer, i Installation, host string) error {
	return PrintSetupProfile(w, i, host, "")
}
func PrintSetupProfile(w io.Writer, i Installation, host, profile string) error {
	s, e := HostSnippetProfile(host, i.Executable, profile)
	if e != nil {
		return e
	}
	_, e = fmt.Fprintln(w, s)
	if e != nil {
		return e
	}
	return nil
}
func DefaultDirectory() (string, error) {
	h, e := os.UserHomeDir()
	if e != nil {
		return "", errors.New("home_unavailable")
	}
	return filepath.Join(h, ".seconded"), nil
}

// QuickSettings is the first-run policy; advanced setup retains explicit choices.
func QuickSettings() Settings {
	s := DefaultSettings()
	day := int64(25000000)
	s.Limits.Day = &day
	return s
}

// HostInstructions accompanies the snippet on stderr so stdout stays valid config.
func HostInstructions(host string) string {
	switch host {
	case "claude-desktop":
		return "For a raw binary, merge the printed mcpServers entry into your user Claude Desktop claude_desktop_config.json. MCPB users: connect through the bundle manifest and do not also merge this snippet (duplicate server). Fully quit Claude Desktop (including its tray/menu-bar process), then reopen Claude Desktop."
	case "claude-code":
		return "Run the printed claude mcp add command in your shell (PowerShell on Windows). Exit the current Claude Code session, then start a new claude session."
	case "gemini":
		return "Merge the printed mcpServers entry into your user ~/.gemini/settings.json. Exit and restart Gemini CLI."
	case "cursor":
		return "Merge the printed mcpServers entry into your user ~/.cursor/mcp.json. Fully quit Cursor, then reopen Cursor."
	case "codex":
		return "Merge the printed table into your user ~/.codex/config.toml. Exit and restart the Codex session."
	default:
		return "Add the printed command and arguments to your user-level MCP host settings, then fully quit and reopen that host."
	}
}

// Switching stages a private copy and a separate ledger before atomically changing
// the selection. The OS entry, newer wallet.key and newer ledger.json are retained.
func (g *Engine) switchWallet(earlier bool) (any, error) {
	unlock, err := g.Files.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	return g.switchWalletLocked(earlier)
}

// The terminal holds the same profile lock through confirmation and selection.
func (g *Engine) switchWalletLocked(earlier bool) (any, error) {
	i, err := readInstallation(g.Files)
	if err != nil {
		return nil, err
	}
	if !addressPattern.MatchString(i.EarlierAddress) {
		return nil, errors.New("earlier_wallet_not_found")
	}
	current, err := LoadVault(g.Store)
	if err != nil {
		return nil, err
	}
	previous := current.Address
	// Preserve the latest policy in the retained wallet before leaving an
	// enrollment slot. A failed selection commit still points to that slot.
	if i.EnrollmentSlot != "" && ((earlier && current.Address != i.EarlierAddress) || (!earlier && current.Address != i.NewerAddress)) {
		name := "wallet.key"
		if current.Address == i.EarlierAddress {
			name = "wallet-" + current.Address + ".key"
		}
		if err := SaveVault(namedWalletStore{g.Files, name}, current); err != nil {
			return nil, err
		}
	}
	destination := current.Settings
	if earlier && current.Address != i.EarlierAddress {
		if i.NewerAddress != "" && i.NewerAddress != current.Address {
			return nil, ErrStorage
		}
		walletFile := "wallet-" + i.EarlierAddress + ".key"
		recovered := namedWalletStore{g.Files, walletFile}
		old, err := LoadVault(recovered)
		if errors.Is(err, ErrNotFound) {
			old, err = LoadVault(g.earlierStore())
			if err != nil {
				return nil, err
			}
			if old.Address != i.EarlierAddress {
				return nil, errors.New("earlier_wallet_changed")
			}
			// The owner retains the active policy when first restoring the earlier key.
			old.Fingerprint, old.Settings, old.Version = i.Fingerprint, current.Settings, i.Version
			if err = SaveVault(recovered, old); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
		if old.Address != i.EarlierAddress {
			return nil, ErrStorage
		}
		if old.Fingerprint != i.Fingerprint || old.Version != i.Version {
			old.PreviousFingerprint, old.Fingerprint, old.Version = old.Fingerprint, i.Fingerprint, i.Version
			if err = SaveVault(recovered, old); err != nil {
				return nil, err
			}
		}
		ledgerFile := "ledger-" + i.EarlierAddress + ".json"
		if _, err = g.Files.Read(ledgerFile); errors.Is(err, ErrNotFound) {
			err = g.Files.Write(ledgerFile, []byte(`{"version":1,"entries":[]}`))
		}
		if err != nil {
			return nil, err
		}
		if err = migrateLegacyPolicy(recovered, &old); err != nil {
			return nil, err
		}
		destination = old.Settings
		i.NewerAddress, i.Address = current.Address, old.Address
		i.Backend, i.WalletFile, i.LedgerFile = "file", walletFile, ledgerFile
		i.EnrollmentSlot = ""
	} else if !earlier && current.Address != i.NewerAddress {
		newer, err := LoadVault(FileStore{g.Files})
		if err != nil {
			return nil, err
		}
		if newer.Address != i.NewerAddress {
			return nil, ErrStorage
		}
		if newer.Fingerprint != i.Fingerprint || newer.Version != i.Version {
			newer.PreviousFingerprint, newer.Fingerprint, newer.Version = newer.Fingerprint, i.Fingerprint, i.Version
			if err = SaveVault(FileStore{g.Files}, newer); err != nil {
				return nil, err
			}
		}
		if err = migrateLegacyPolicy(FileStore{g.Files}, &newer); err != nil {
			return nil, err
		}
		destination = newer.Settings
		i.Address, i.Backend, i.WalletFile, i.LedgerFile = newer.Address, "file", "", ""
		i.EnrollmentSlot = ""
	}
	// Validate the destination before committing selection, including retained ledgers.
	ledgerFile := i.LedgerFile
	if ledgerFile == "" {
		ledgerFile = "ledger.json"
	}
	ledger, err := readLedgerFile(g.Files, ledgerFile)
	if err != nil {
		return nil, err
	}
	for _, entry := range ledger.Entries {
		if entry.Payer != i.Address {
			return nil, ErrStorage
		}
	}
	var audit policyAudit
	changed := previous != i.Address
	if changed {
		change := newPolicyChange(current.Settings, destination, previous, i.Address, "human terminal wallet switch")
		audit, err = g.preparePolicyLocked(current, change)
		if err != nil {
			return nil, err
		}
		i.PolicyRevision = change.ID
	}
	i.EarlierNoticePending = false
	if err = saveInstallation(g.Files, i); err != nil {
		return nil, err
	}
	if changed {
		audit.Changes[len(audit.Changes)-1].Applied = true
		if err = audit.save(g.Files); err != nil {
			return nil, err
		}
	}
	return map[string]any{"status": "wallet_switched", "address": i.Address, "previous_address": previous, "earlier_address": i.EarlierAddress, "newer_address": i.NewerAddress, "message": "Active SECONDED wallet: " + i.Address + ". Earlier wallet: " + i.EarlierAddress + "; newer wallet: " + i.NewerAddress + ". Both keys and their separate payment records are retained."}, nil
}

func (g *Engine) takeEarlierWalletNotice(ctx context.Context) (string, error) {
	if g == nil || ctx.Err() != nil {
		return "", nil
	}
	unlock, err := g.Files.Lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	i, err := readInstallation(g.Files)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil || !i.EarlierNoticePending {
		return "", err
	}
	if !addressPattern.MatchString(i.EarlierAddress) {
		return "", ErrStorage
	}
	balances, _ := g.walletBalances(ctx, i.EarlierAddress)
	if ctx.Err() != nil {
		return "", nil
	}
	parts := make([]string, 0, len(balances))
	for _, balance := range balances {
		amount := "balance unknown"
		if balance.Balance != nil {
			amount = *balance.Balance + " " + balance.Token
		}
		parts = append(parts, balance.Name+": "+amount)
	}
	notice := "Found your earlier SECONDED wallet " + i.EarlierAddress + " with " + strings.Join(parts, "; ") + " in it. Use seconded-mcp wallet --human --switch back in your terminal to select it."
	i.EarlierNoticePending = false
	if err = saveInstallation(g.Files, i); err != nil {
		return "", err
	}
	return notice, nil
}
