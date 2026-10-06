//go:build darwin

package client

import (
	"errors"
	"io"
	"strings"
	"testing"
	"unsafe"

	"github.com/zalando/go-keyring"
)

func TestNativeCredentialStatusDoesNotInventMissingWallet(t *testing.T) {
	for _, status := range []int32{0, -25300, -25293, -25308, -50, -128} {
		err := nativeCredentialStatus(status)
		if status == 0 {
			if err != nil {
				t.Fatal(err)
			}
		} else if status == -25300 {
			if !errors.Is(err, keyring.ErrNotFound) {
				t.Fatal(err)
			}
		} else if !errors.Is(err, ErrKeystoreUnavailable) {
			t.Fatal("denial became absence", status, err)
		}
	}
}

func installACLFixture(t *testing.T, api *keychainACLAPI) *keychainAPI {
	t.Helper()
	base := noPromptAPI(t)
	base.release = func(uintptr) {}
	base.command = func([]string, string) ([]byte, int) {
		t.Fatal("native credential used security subprocess")
		return nil, 1
	}
	oldBase, oldACL := nativeKeychain, nativeACL
	nativeKeychain = func() (*keychainAPI, error) { return base, nil }
	nativeACL = func() (*keychainACLAPI, error) { return api, nil }
	t.Cleanup(func() { nativeKeychain, nativeACL = oldBase, oldACL })
	return base
}

func TestHardenedCredentialReadPositiveControlAndDenial(t *testing.T) {
	data := []byte("go-keyring-base64:Zml4dHVyZQ==")
	status := int32(0)
	freed := 0
	api := &keychainACLAPI{find: func(_ uintptr, _ uint32, service string, _ uint32, account string, n *uint32, p *unsafe.Pointer, _ *uintptr) int32 {
		if service != "seconded" || account != "vault" {
			t.Fatal("wrong item")
		}
		if status == 0 {
			*n = uint32(len(data))
			*p = unsafe.Pointer(&data[0])
		}
		return status
	}, free: func(uintptr, unsafe.Pointer) int32 { freed++; return 0 }}
	installACLFixture(t, api)
	got, err := hardenedKeychainGet("/fixture.keychain-db", "seconded", "vault")
	if err != nil || got != "fixture" || freed != 1 {
		t.Fatal("native read positive control", err, freed)
	}
	for _, denied := range []int32{-25293, -25308, -25300} {
		status = denied
		_, err = hardenedKeychainGet("/fixture.keychain-db", "seconded", "vault")
		if err == nil || (denied != -25300 && !errors.Is(err, ErrKeystoreUnavailable)) {
			t.Fatal(err)
		}
	}
}

func TestHardenedCredentialCreationTrustsOnlyCreator(t *testing.T) {
	created := false
	api := &keychainACLAPI{
		find:         func(uintptr, uint32, string, uint32, string, *uint32, *unsafe.Pointer, *uintptr) int32 { return -25300 },
		stringCreate: func(uintptr, string, uint32) uintptr { return 7 },
		access: func(_ uintptr, trust uintptr, out *uintptr) int32 {
			if trust != 0 {
				t.Fatal("extra trusted applications")
			}
			*out = 8
			return 0
		},
		create: func(class uint32, attrs *keychainAttributes, n uint32, data unsafe.Pointer, keychain, access uintptr, item *uintptr) int32 {
			if class != 0x67656e70 || attrs.Count != 2 || keychain != 42 || access != 8 || string(unsafe.Slice((*byte)(data), n)) != "go-keyring-base64:Zml4dHVyZQ==" {
				t.Fatal("wrong creation contract")
			}
			created = true
			*item = 9
			return 0
		},
	}
	installACLFixture(t, api)
	if err := hardenedKeychainSet("/fixture.keychain-db", "seconded", "vault", "fixture"); err != nil || !created {
		t.Fatal(err)
	}
	api.find = func(uintptr, uint32, string, uint32, string, *uint32, *unsafe.Pointer, *uintptr) int32 { return -25293 }
	created = false
	if err := hardenedKeychainSet("/fixture.keychain-db", "seconded", "vault", "fixture"); !errors.Is(err, ErrKeystoreUnavailable) || created {
		t.Fatal("denial created replacement", err)
	}
}

func TestACLMigrationCoversEveryServiceItemAndVerifiesReadback(t *testing.T) {
	if !keychainACLCandidate {
		t.Skip("candidate build required")
	}
	for _, badReadback := range []bool{false, true} {
		t.Run(map[bool]string{false: "all copies", true: "readback failure"}[badReadback], func(t *testing.T) {
			oldPresence := authenticateOwnerPresence
			authenticateOwnerPresence = func(string) error { return nil }
			t.Cleanup(func() { authenticateOwnerPresence = oldPresence })
			migrated := map[uintptr]bool{}
			cursor := uintptr(10)
			allowed := false
			good, bad := []byte("same secret"), []byte("corrupt")
			api := &keychainACLAPI{
				stringCreate: func(uintptr, string, uint32) uintptr { return 7 }, access: func(_ uintptr, trust uintptr, p *uintptr) int32 {
					if trust != 0 {
						t.Fatal("not creator only")
					}
					*p = 8
					return 0
				},
				search: func(_ uintptr, class uint32, attrs *keychainAttributes, p *uintptr) int32 {
					if class != 0x67656e70 || attrs.Count != 1 || attrs.Attributes.Tag != 0x73727663 || string(unsafe.Slice((*byte)(attrs.Attributes.Data), attrs.Attributes.Length)) != "seconded" {
						t.Fatal("search omitted retained/profile items")
					}
					*p = 9
					return 0
				},
				next: func(_ uintptr, p *uintptr) int32 {
					if cursor == 13 {
						return -25300
					}
					*p = cursor
					cursor++
					return 0
				},
				setAccess: func(item, access uintptr) int32 {
					if !allowed || access != 8 {
						t.Fatal("migration unapproved")
					}
					migrated[item] = true
					return 0
				},
				copyData: func(item uintptr, _, _, _ uintptr, n *uint32, p *unsafe.Pointer) int32 {
					data := good
					if migrated[item] {
						if allowed {
							t.Fatal("readback allowed UI")
						}
						if badReadback {
							data = bad
						}
					}
					*n = uint32(len(data))
					*p = unsafe.Pointer(&data[0])
					return 0
				},
				freeData: func(uintptr, unsafe.Pointer) int32 { return 0 },
			}
			base := installACLFixture(t, api)
			noUI := base.interaction
			base.interaction = func(v bool) int32 { noUI(false); allowed = v; return 0 }
			count, err := MigrateKeychainACL("/fixture.keychain-db", strings.NewReader(""), io.Discard)
			if allowed {
				t.Fatal("migration left UI enabled")
			}
			if badReadback {
				if !errors.Is(err, ErrKeystoreData) || count != 0 {
					t.Fatal("readback mismatch accepted", err, count)
				}
			} else if err != nil || count != 3 || len(migrated) != 3 {
				t.Fatal("not all service entries migrated", err, count)
			}
		})
	}
}
