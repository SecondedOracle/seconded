package client

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"path/filepath"

	"github.com/zalando/go-keyring"
)

var ErrStorage = errors.New("local_storage_unavailable")
var ErrNotFound = errors.New("not_found")
var ErrKeystoreUnavailable = errors.New("keystore_unavailable")
var ErrKeystoreData = errors.New("keystore_data_invalid")

const KeystoreDataMessage = "The OS credential store returned invalid or inconsistent wallet data. Preserve the existing wallet and its backups; restore a valid credential before retrying. Changing profile permissions will not repair it."

// StorageErrorMessage exposes only fixed guidance or a validated profile path.
func StorageErrorMessage(err error) string {
	if errors.Is(err, ErrKeystoreData) {
		return KeystoreDataMessage
	}
	var storageError *walletStorageError
	if errors.As(err, &storageError) {
		return storageError.Error()
	}
	return "Local storage is unavailable. Ensure the profile is writable, private and owned by you."
}

// StorageFailureMessage contains only an owner-visible path, never backend output.
func StorageFailureMessage(dir string) string {
	return fmt.Sprintf("Cannot safely write wallet storage in %s. Use a writable profile directory owned by you with private permissions (0700 directory, 0600 files on Unix).", dir)
}

type walletStorageError struct{ dir string }

func (e *walletStorageError) Error() string { return StorageFailureMessage(e.dir) }
func (e *walletStorageError) Unwrap() error { return ErrStorage }

func setupStorageError(err error, dir string) error {
	if errors.Is(err, ErrStorage) {
		return &walletStorageError{dir: dir}
	}
	return err
}

type SecretStore interface {
	Get(string) (string, error)
	Set(string, string) error
}
type OSStore struct{}

func (OSStore) Get(k string) (string, error) {
	if path := os.Getenv("SECONDED_KEYCHAIN_PATH"); path != "" {
		return readOSSecret(explicitKeychain{path}.get, k)
	}
	return readOSSecret(func(service, account string) (string, error) {
		return platformKeychainGet("", service, account)
	}, k)
}

func readOSSecret(get func(string, string) (string, error), k string) (string, error) {
	s, e := get("seconded", k)
	if errors.Is(e, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if errors.Is(e, ErrKeystoreData) {
		return "", ErrKeystoreData
	}
	var badHex hex.InvalidByteError
	var badBase64 base64.CorruptInputError
	if errors.As(e, &badHex) || errors.As(e, &badBase64) || errors.Is(e, hex.ErrLength) {
		return "", ErrKeystoreData
	}
	if e != nil {
		return "", ErrKeystoreUnavailable
	}
	return s, nil
}
func (OSStore) Set(k, v string) error {
	if path := os.Getenv("SECONDED_KEYCHAIN_PATH"); path != "" {
		store := explicitKeychain{path}
		return writeOSSecret(store.get, store.set, k, v)
	}
	return writeOSSecret(func(service, account string) (string, error) {
		return platformKeychainGet("", service, account)
	}, func(service, account, value string) error {
		return platformKeychainSet("", service, account, value)
	}, k, v)
}

func writeOSSecret(get func(string, string) (string, error), set func(string, string, string) error, k, v string) error {
	if e := set("seconded", k, v); e != nil {
		if errors.Is(e, keyring.ErrSetDataTooBig) {
			return ErrKeystoreData
		}
		return ErrKeystoreUnavailable
	}
	// Backend success alone does not prove the credential was persisted.
	// Enrollment is complete only when the same credential can be retrieved.
	saved, e := readOSSecret(get, k)
	if e != nil {
		if errors.Is(e, ErrKeystoreData) {
			return e
		}
		return ErrKeystoreUnavailable
	}
	if saved != v {
		return ErrKeystoreData
	}
	return nil
}

// An explicit keychain is useful for isolated macOS profiles and acceptance tests.
// Every credential operation names it; default keychains and search lists are untouched.
type explicitKeychain struct{ path string }

func (s explicitKeychain) valid() bool {
	if runtime.GOOS != "darwin" || !filepath.IsAbs(s.path) {
		return false
	}
	info, err := os.Lstat(s.path)
	return err == nil && info.Mode().IsRegular()
}
func (s explicitKeychain) get(service, account string) (string, error) {
	if !s.valid() {
		return "", ErrKeystoreUnavailable
	}
	return platformKeychainGet(s.path, service, account)
}

func decodeKeychainValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "go-keyring-base64:") {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "go-keyring-base64:"))
		return string(raw), err
	}
	if strings.HasPrefix(value, "go-keyring-encoded:") {
		raw, err := hex.DecodeString(strings.TrimPrefix(value, "go-keyring-encoded:"))
		return string(raw), err
	}
	return value, nil
}
func (s explicitKeychain) set(service, account, value string) error {
	if !s.valid() {
		return ErrKeystoreUnavailable
	}
	return platformKeychainSet(s.path, service, account, value)
}

// ProfileStore keeps a custom profile away from the default OS vault entry.
// The directory is part of the identity: moving it requires explicit recovery.
type ProfileStore struct {
	Store   SecretStore
	Profile string
}

func (s ProfileStore) key(k string) string {
	h := sha256.Sum256([]byte(filepath.Clean(s.Profile)))
	return "profile-" + hex.EncodeToString(h[:]) + "-" + k
}
func (s ProfileStore) Get(k string) (string, error) { return s.Store.Get(s.key(k)) }
func (s ProfileStore) Set(k, v string) error        { return s.Store.Set(s.key(k), v) }

func ProfileOSStore(dir string) SecretStore {
	defaultDir, err := DefaultDirectory()
	if err == nil && filepath.Clean(dir) == filepath.Clean(defaultDir) {
		return OSStore{}
	}
	return ProfileStore{Store: OSStore{}, Profile: dir}
}

type Files struct{ Dir string }

func OpenFiles(dir string) (*Files, error) {
	if !filepath.IsAbs(dir) {
		return nil, ErrStorage
	}
	if e := createPrivateDirectory(dir); e != nil {
		return nil, ErrStorage
	}
	f := &Files{dir}
	if e := securePath(dir, true); e != nil {
		return nil, e
	}
	return f, nil
}
func (f *Files) Read(name string) ([]byte, error) {
	p := filepath.Join(f.Dir, name)
	if filepath.Base(name) != name {
		return nil, ErrStorage
	}
	if e := securePath(f.Dir, true); e != nil {
		return nil, e
	}
	if e := securePath(p, false); e != nil {
		return nil, e
	}
	in, e := os.Open(p)
	if e != nil {
		return nil, ErrStorage
	}
	defer in.Close()
	b, e := io.ReadAll(io.LimitReader(in, 32<<20))
	if e != nil || len(b) >= 32<<20 {
		return nil, ErrStorage
	}
	return b, nil
}
func (f *Files) Write(name string, b []byte) error {
	if filepath.Base(name) != name || len(b) > 32<<20 {
		return ErrStorage
	}
	if e := securePath(f.Dir, true); e != nil {
		return e
	}
	p := filepath.Join(f.Dir, name)
	if e := securePath(p, false); e != nil && !errors.Is(e, ErrNotFound) {
		return e
	}
	out, e := os.CreateTemp(f.Dir, ".seconded-*")
	if e != nil {
		return ErrStorage
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	if e = out.Chmod(0600); e != nil {
		out.Close()
		return ErrStorage
	}
	if e = securePath(tmp, false); e != nil {
		out.Close()
		return e
	}
	if _, e = out.Write(b); e != nil {
		out.Close()
		return ErrStorage
	}
	if e = out.Sync(); e != nil {
		out.Close()
		return ErrStorage
	}
	if e = out.Close(); e != nil {
		return ErrStorage
	}
	if e = replaceFile(tmp, p); e != nil {
		return ErrStorage
	}
	return syncDir(f.Dir)
}
func (f *Files) Lock() (func(), error) {
	if e := securePath(f.Dir, true); e != nil {
		return nil, e
	}
	p := filepath.Join(f.Dir, "lock")
	if e := securePath(p, false); e != nil && !errors.Is(e, ErrNotFound) {
		return nil, e
	}
	fd, e := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, ErrStorage
	}
	if e = lockFile(fd); e != nil {
		fd.Close()
		return nil, ErrStorage
	}
	return func() { unlockFile(fd); fd.Close() }, nil
}

type FileStore struct{ Files *Files }

func (s FileStore) Get(k string) (string, error) {
	if k != "vault" {
		return "", ErrInvalid
	}
	b, e := s.Files.Read("wallet.key")
	return string(b), e
}
func (s FileStore) Set(k, v string) error {
	if k != "vault" {
		return ErrInvalid
	}
	return s.Files.Write("wallet.key", []byte(v))
}

type Vault struct {
	PolicyVersion       int      `json:"policy_version,omitempty"`
	Version             string   `json:"version,omitempty"`
	PreviousFingerprint string   `json:"previous_fingerprint,omitempty"`
	PolicyRevision      string   `json:"policy_revision,omitempty"`
	Key                 string   `json:"key"`
	Address             string   `json:"address"`
	Settings            Settings `json:"settings"`
	Fingerprint         string   `json:"fingerprint"`
}

func LoadVault(s SecretStore) (Vault, error) {
	var v Vault
	raw, e := s.Get("vault")
	if e != nil {
		return v, e
	}
	if e = DecodeStrict([]byte(raw), &v, ResponseLimit); e != nil {
		return v, vaultDataError(s)
	}
	if !hexDigest.MatchString(v.Key) || !hexDigest.MatchString(v.Fingerprint) {
		return v, vaultDataError(s)
	}
	if v.Settings.Validate() != nil {
		return v, vaultDataError(s)
	}
	b, _ := hex.DecodeString(v.Key)
	signer, e := NewLocalSigner(b)
	clear(b)
	if e != nil {
		return v, vaultDataError(s)
	}
	defer signer.Close()
	if signer.Address() != v.Address {
		return v, vaultDataError(s)
	}
	return v, nil
}

// Invalid OS-vault content needs credential repair, not profile permissions.
func vaultDataError(s SecretStore) error {
	switch store := s.(type) {
	case OSStore, ProfileStore, enrollmentSecret:
		return ErrKeystoreData
	case activeWalletStore:
		if i, err := readInstallation(store.files); err == nil && i.Backend == "os_keystore" {
			return ErrKeystoreData
		}
	}
	return ErrStorage
}

func SaveVault(s SecretStore, v Vault) error {
	b, e := json.Marshal(v)
	if e != nil {
		return ErrStorage
	}
	return s.Set("vault", string(b))
}

// NativeOwner is a compatibility no-op and never invokes OS authentication.
type NativeOwner struct{}

func (NativeOwner) Confirm([]string) error { return nil }

// OwnerConfirmation is retained for source compatibility; policy changes never call it.
type OwnerConfirmation interface{ Confirm(changes []string) error }

// ChangeSettings is the legacy CLI entry point. Its caller holds the profile lock.
// New callers should use Engine.Limits or TerminalLimits, which take that lock.
// A callback is not evidence of a controlling terminal.
func ChangeSettings(store SecretStore, next Settings, _ OwnerConfirmation, auditFiles ...*Files) error {
	if e := next.Validate(); e != nil {
		return e
	}
	v, e := LoadVault(store)
	if e != nil {
		return e
	}
	if e = ChatPolicy(v.Settings, next); e != nil {
		return e
	}
	var files *Files
	switch s := store.(type) {
	case activeWalletStore:
		files = s.files
	case FileStore:
		files = s.Files
	case namedWalletStore:
		files = s.files
	case interface{ PolicyFiles() *Files }:
		files = s.PolicyFiles()
	}
	if len(auditFiles) == 1 {
		files = auditFiles[0]
	}
	if files == nil {
		return ErrStorage
	}
	return (&Engine{Files: files, Store: store}).savePolicyLocked(v, next, "chat")
}

type Installation struct {
	Version              string `json:"version,omitempty"`
	PreviousFingerprint  string `json:"previous_fingerprint,omitempty"`
	EnrollmentSlot       string `json:"enrollment_slot,omitempty"`
	PolicyRevision       string `json:"policy_revision,omitempty"`
	Address              string `json:"address"`
	Backend              string `json:"backend"`
	Fingerprint          string `json:"fingerprint"`
	Executable           string `json:"executable"`
	EarlierCheckPending  bool   `json:"earlier_check_pending,omitempty"`
	EarlierAddress       string `json:"earlier_address,omitempty"`
	EarlierNoticePending bool   `json:"earlier_notice_pending,omitempty"`
	NewerAddress         string `json:"newer_address,omitempty"`
	WalletFile           string `json:"wallet_file,omitempty"`
	LedgerFile           string `json:"ledger_file,omitempty"`
}

func Fingerprint(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", ErrStorage
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", ErrStorage
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func CheckFingerprint(files *Files, store SecretStore, exe string) error {
	unlock, err := files.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	return checkEnrollmentLocked(files, store, exe, false)

}

// releaseDigest is the authenticated manifest or owner-supplied SHA-256; no wallet is created or
// reopened by a binary whose bytes do not match it.
func SetupWallet(files *Files, osStore SecretStore, _ bool, settings Settings, exe, releaseDigest string) (install Installation, err error) {
	defer func() { err = setupStorageError(err, files.Dir) }()
	var i Installation
	if settings.Validate() != nil {
		return i, ErrInvalid
	}
	h, e := Fingerprint(exe)
	if e != nil {
		return i, e
	}
	if !hexDigest.MatchString(releaseDigest) || h != releaseDigest {
		return i, errors.New("release_digest_mismatch")
	}
	unlock, e := files.Lock()
	if e != nil {
		return i, e
	}
	defer unlock()
	if b, e := files.Read("install.json"); e == nil {
		if DecodeStrict(b, &i, ResponseLimit) != nil {
			return i, ErrStorage
		}
		store := osStore
		if i.Backend == "file" {
			store = activeWalletStore{files, osStore}
		} else if i.Backend != "os_keystore" {
			return i, ErrStorage
		}
		if err := checkEnrollmentLocked(files, store, exe, false); err != nil {
			return i, err
		}
		// Another setup may have completed while this caller verified the release.
		// Never return success for a newly proposed policy that was not installed.
		return i, errors.New("wallet_already_exists")
	} else if !errors.Is(e, ErrNotFound) {
		return i, e
	}
	// Missing install metadata must not silently replace an existing wallet.
	_, storeErr := osStore.Get("vault")
	if storeErr == nil {
		return i, errors.New("wallet_already_exists")
	}
	if _, e := files.Read("wallet.key"); e == nil {
		return i, errors.New("wallet_already_exists")
	} else if !errors.Is(e, ErrNotFound) {
		return i, e
	}
	if _, e := files.Read("ledger.json"); e == nil {
		return i, errors.New("wallet_already_exists")
	} else if !errors.Is(e, ErrNotFound) {
		return i, e
	}
	raw, e := GenerateKey()
	if e != nil {
		return i, e
	}
	defer clear(raw)
	signer, e := NewLocalSigner(raw)
	if e != nil {
		return i, e
	}
	defer signer.Close()
	v := Vault{PolicyVersion: 1, Version: Version, Key: hex.EncodeToString(raw), Address: signer.Address(), Settings: settings, Fingerprint: h}
	backend := "os_keystore"
	if errors.Is(storeErr, ErrNotFound) {
		e = SaveVault(osStore, v)
	} else {
		e = storeErr
	}
	if e != nil {
		// The legacy flag remains source-compatible. Automatic fallback requires
		// an unavailable credential service, not malformed or conflicting data.
		if !errors.Is(e, ErrKeystoreUnavailable) {
			return i, e
		}
		if e = SaveVault(FileStore{files}, v); e != nil {
			return i, e
		}
		backend = "file"
	}
	i = Installation{Version: Version, Address: v.Address, Backend: backend, Fingerprint: h, Executable: exe, EarlierCheckPending: errors.Is(storeErr, ErrKeystoreUnavailable)}
	b, _ := json.Marshal(i)
	if e = files.Write("ledger.json", []byte(`{"version":1,"entries":[]}`)); e != nil {
		return i, e
	}
	if e = files.Write("install.json", b); e != nil {
		return i, e
	}
	return i, nil
}

func readInstallation(files *Files) (Installation, error) {
	var i Installation
	raw, err := files.Read("install.json")
	if err != nil {
		return i, err
	}
	if DecodeStrict(raw, &i, ResponseLimit) != nil {
		return i, ErrStorage
	}
	return i, nil
}

func saveInstallation(files *Files, i Installation) error {
	raw, err := json.Marshal(i)
	if err != nil {
		return ErrStorage
	}
	return files.Write("install.json", raw)
}

func walletFilename(i Installation) string {
	if i.EnrollmentSlot != "" {
		return "wallet-enrollment-" + i.EnrollmentSlot + ".key"
	}
	if i.WalletFile != "" {
		return i.WalletFile
	}
	return "wallet.key"
}

// Resolve selection on every operation so already-running hosts follow a switch.
// Callers hold the profile lock across wallet/ledger transactions.
type activeWalletStore struct {
	files   *Files
	osStore SecretStore
}

type namedWalletStore struct {
	files *Files
	name  string
}

func (s namedWalletStore) Get(k string) (string, error) {
	if k != "vault" {
		return "", ErrInvalid
	}
	raw, err := s.files.Read(s.name)
	return string(raw), err
}
func (s namedWalletStore) Set(k, value string) error {
	if k != "vault" {
		return ErrInvalid
	}
	return s.files.Write(s.name, []byte(value))
}

func (s activeWalletStore) selected() (SecretStore, Installation, error) {
	i, err := readInstallation(s.files)
	if err != nil {
		return nil, i, err
	}
	if i.EnrollmentSlot != "" {
		store, err := enrollmentStore(s.files, s.osStore, i)
		return store, i, err
	}
	if i.Backend == "os_keystore" && i.WalletFile == "" {
		return s.osStore, i, nil
	}
	if i.Backend != "file" {
		return nil, i, ErrStorage
	}
	if i.WalletFile != "" && (!addressPattern.MatchString(i.EarlierAddress) || i.Address != i.EarlierAddress || i.WalletFile != "wallet-"+i.EarlierAddress+".key") {
		return nil, i, ErrStorage
	}
	return namedWalletStore{s.files, walletFilename(i)}, i, nil
}
func (s activeWalletStore) Get(k string) (string, error) {
	store, i, err := s.selected()
	if err != nil {
		return "", err
	}
	raw, err := store.Get(k)
	if err != nil {
		return "", err
	}
	var v Vault
	if DecodeStrict([]byte(raw), &v, ResponseLimit) != nil || v.Address != i.Address || v.Fingerprint != i.Fingerprint || v.Version != i.Version {
		return "", vaultDataError(s)
	}
	return raw, nil
}
func (s activeWalletStore) Set(k, raw string) error {
	store, i, err := s.selected()
	if err != nil {
		return err
	}
	var v Vault
	if DecodeStrict([]byte(raw), &v, ResponseLimit) != nil || v.Address != i.Address || v.Fingerprint != i.Fingerprint || v.Version != i.Version {
		return ErrStorage
	}
	return store.Set(k, raw)
}

func (g *Engine) earlierStore() SecretStore {
	if store, ok := g.Store.(activeWalletStore); ok {
		return store.osStore
	}
	return ProfileOSStore(g.Files.Dir)
}

// Discovery retains only public metadata. A locked store leaves the retry pending.
func (g *Engine) discoverEarlierWallet(ctx context.Context) error {
	if g == nil {
		return nil
	}
	unlock, err := g.Files.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	i, err := readInstallation(g.Files)
	// Library engines used without setup have no recovery state.
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil || !i.EarlierCheckPending {
		return err
	}
	if ctx.Err() != nil {
		return nil
	}
	old, err := LoadVault(g.earlierStore())
	if errors.Is(err, ErrKeystoreUnavailable) {
		return nil
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		if errors.Is(err, ErrStorage) {
			return ErrKeystoreData
		}
		return err
	}
	i.EarlierCheckPending = false
	if err == nil && old.Address != i.Address {
		i.EarlierAddress = old.Address
		i.NewerAddress = i.Address
		i.EarlierNoticePending = true
	}
	return saveInstallation(g.Files, i)
}
