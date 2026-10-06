package client

import (
	"bytes"
	"encoding/base64"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/zalando/go-keyring"
)

// The candidate uses in-process Keychain Services for data as well as metadata.
// It deliberately has no security(1) fallback on denied native access.
type keychainAttribute struct {
	Tag, Length uint32
	Data        unsafe.Pointer
}
type keychainAttributes struct {
	Count      uint32
	Attributes *keychainAttribute
}
type keychainACLAPI struct {
	find         func(uintptr, uint32, string, uint32, string, *uint32, *unsafe.Pointer, *uintptr) int32
	free         func(uintptr, unsafe.Pointer) int32
	modify       func(uintptr, uintptr, uint32, unsafe.Pointer) int32
	create       func(uint32, *keychainAttributes, uint32, unsafe.Pointer, uintptr, uintptr, *uintptr) int32
	access       func(uintptr, uintptr, *uintptr) int32
	stringCreate func(uintptr, string, uint32) uintptr
	setAccess    func(uintptr, uintptr) int32
	search       func(uintptr, uint32, *keychainAttributes, *uintptr) int32
	next         func(uintptr, *uintptr) int32
	copyData     func(uintptr, uintptr, uintptr, uintptr, *uint32, *unsafe.Pointer) int32
	freeData     func(uintptr, unsafe.Pointer) int32
}

var nativeACL = sync.OnceValues(func() (api *keychainACLAPI, err error) {
	defer func() {
		if recover() != nil {
			api = nil
			err = ErrKeystoreUnavailable
		}
	}()
	handle, err := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, ErrKeystoreUnavailable
	}
	api = &keychainACLAPI{}
	purego.RegisterLibFunc(&api.find, handle, "SecKeychainFindGenericPassword")
	purego.RegisterLibFunc(&api.free, handle, "SecKeychainItemFreeContent")
	purego.RegisterLibFunc(&api.modify, handle, "SecKeychainItemModifyAttributesAndData")
	purego.RegisterLibFunc(&api.create, handle, "SecKeychainItemCreateFromContent")
	purego.RegisterLibFunc(&api.access, handle, "SecAccessCreate")
	purego.RegisterLibFunc(&api.stringCreate, handle, "CFStringCreateWithCString")
	purego.RegisterLibFunc(&api.setAccess, handle, "SecKeychainItemSetAccess")
	purego.RegisterLibFunc(&api.search, handle, "SecKeychainSearchCreateFromAttributes")
	purego.RegisterLibFunc(&api.next, handle, "SecKeychainSearchCopyNext")
	purego.RegisterLibFunc(&api.copyData, handle, "SecKeychainItemCopyAttributesAndData")
	purego.RegisterLibFunc(&api.freeData, handle, "SecKeychainItemFreeAttributesAndData")
	return api, nil
})

func nativeCredentialStatus(status int32) error {
	if status == 0 {
		return nil
	}
	if status == -25300 {
		return keyring.ErrNotFound
	}
	return ErrKeystoreUnavailable
}

func withNativeCredential(path string, operation func(*keychainAPI, *keychainACLAPI, uintptr) error) error {
	base, err := nativeKeychain()
	if err != nil {
		return err
	}
	api, err := nativeACL()
	if err != nil {
		return err
	}
	return base.withKeychain(path, func(target string) error {
		var ref uintptr
		if base.open(target, &ref) != 0 || ref == 0 {
			return ErrKeystoreUnavailable
		}
		defer base.release(ref)
		return operation(base, api, ref)
	})
}

func hardenedKeychainGet(path, service, account string) (string, error) {
	var value string
	err := withNativeCredential(path, func(base *keychainAPI, api *keychainACLAPI, ref uintptr) error {
		var size uint32
		var data unsafe.Pointer
		status := api.find(ref, uint32(len(service)), service, uint32(len(account)), account, &size, &data, nil)
		if err := nativeCredentialStatus(status); err != nil {
			return err
		}
		defer api.free(0, data)
		if size > ResponseLimit || (size > 0 && data == nil) {
			return ErrKeystoreData
		}
		value = string(unsafe.Slice((*byte)(data), int(size)))
		return nil
	})
	if err != nil {
		return "", err
	}
	return decodeKeychainValue(value)
}

func credentialAttributes(service, account []byte) ([]keychainAttribute, keychainAttributes) {
	attrs := []keychainAttribute{{0x73727663, uint32(len(service)), unsafe.Pointer(unsafe.SliceData(service))}, {0x61636374, uint32(len(account)), unsafe.Pointer(unsafe.SliceData(account))}}
	return attrs, keychainAttributes{uint32(len(attrs)), &attrs[0]}
}

func creatorAccess(base *keychainAPI, api *keychainACLAPI) (uintptr, error) {
	label := api.stringCreate(0, "SECONDED wallet — approved client only", 0x08000100)
	if label == 0 {
		return 0, ErrKeystoreUnavailable
	}
	defer base.release(label)
	var access uintptr
	// NULL trustedlist means only this executable; never /usr/bin/security.
	if api.access(label, 0, &access) != 0 || access == 0 {
		return 0, ErrKeystoreUnavailable
	}
	return access, nil
}

func hardenedKeychainSet(path, service, account, value string) error {
	if len(value) > ResponseLimit || strings.ContainsRune(service+account, 0) {
		return ErrKeystoreData
	}
	encoded := []byte("go-keyring-base64:" + base64.StdEncoding.EncodeToString([]byte(value)))
	defer clear(encoded)
	return withNativeCredential(path, func(base *keychainAPI, api *keychainACLAPI, ref uintptr) error {
		var item uintptr
		status := api.find(ref, uint32(len(service)), service, uint32(len(account)), account, nil, nil, &item)
		if status == 0 {
			defer base.release(item)
			// Updating an existing item preserves its ACL. Migration is explicit.
			if api.modify(item, 0, uint32(len(encoded)), unsafe.Pointer(unsafe.SliceData(encoded))) != 0 {
				return ErrKeystoreUnavailable
			}
			return nil
		}
		if status != -25300 {
			return ErrKeystoreUnavailable
		}
		access, err := creatorAccess(base, api)
		if err != nil {
			return err
		}
		defer base.release(access)
		sb, ab := []byte(service), []byte(account)
		attrs, list := credentialAttributes(sb, ab)
		status = api.create(0x67656e70, &list, uint32(len(encoded)), unsafe.Pointer(unsafe.SliceData(encoded)), ref, access, &item)
		runtime.KeepAlive(attrs)
		runtime.KeepAlive(sb)
		runtime.KeepAlive(ab)
		runtime.KeepAlive(encoded)
		if item != 0 {
			base.release(item)
		}
		if status != 0 {
			return ErrKeystoreUnavailable
		}
		return nil
	})
}

func copyCredentialData(api *keychainACLAPI, item uintptr) ([]byte, error) {
	var size uint32
	var data unsafe.Pointer
	if api.copyData(item, 0, 0, 0, &size, &data) != 0 {
		return nil, ErrKeystoreUnavailable
	}
	defer api.freeData(0, data)
	if size > ResponseLimit || (size > 0 && data == nil) {
		return nil, ErrKeystoreData
	}
	return append([]byte(nil), unsafe.Slice((*byte)(data), int(size))...), nil
}

// MigrateKeychainACL is an explicit candidate-only owner operation, never an MCP
// entry point or automatic enrollment fallback. It migrates ALL seconded service
// items in the specified keychain, including legacy/profile/retained slots. No
// item or data is deleted. A partial failure is retryable and reported as failure.
func MigrateKeychainACL(path string, input io.Reader, output io.Writer) (int, error) {
	if !keychainACLCandidate || !filepath.IsAbs(path) {
		return 0, ErrKeystoreUnavailable
	}
	if err := ConfirmOwnerPresence("Restrict every SECONDED credential in keychain "+path+" to this client executable", input, output); err != nil {
		return 0, err
	}
	count := 0
	err := withNativeCredential(path, func(base *keychainAPI, api *keychainACLAPI, ref uintptr) error {
		service := []byte("seconded")
		attr := keychainAttribute{0x73727663, uint32(len(service)), unsafe.Pointer(unsafe.SliceData(service))}
		list := keychainAttributes{1, &attr}
		var search uintptr
		status := api.search(ref, 0x67656e70, &list, &search)
		runtime.KeepAlive(service)
		if status != 0 || search == 0 {
			return ErrKeystoreUnavailable
		}
		defer base.release(search)
		access, err := creatorAccess(base, api)
		if err != nil {
			return err
		}
		defer base.release(access)
		// Only this owner operation may show migration/ACL permission dialogs.
		if base.interaction(true) != 0 {
			return ErrKeystoreUnavailable
		}
		defer base.interaction(false)
		for {
			var item uintptr
			status = api.next(search, &item)
			if status == -25300 {
				return nil
			}
			if status != 0 || item == 0 {
				return ErrKeystoreUnavailable
			}
			err = func() error {
				defer base.release(item)
				before, err := copyCredentialData(api, item)
				if err != nil {
					return err
				}
				defer clear(before)
				if api.setAccess(item, access) != 0 {
					return ErrKeystoreUnavailable
				}
				// Read back WITHOUT UI to prove this executable retained silent access.
				if base.interaction(false) != 0 {
					return ErrKeystoreUnavailable
				}
				after, readErr := copyCredentialData(api, item)
				defer clear(after)
				if readErr != nil || !bytes.Equal(before, after) {
					return ErrKeystoreData
				}
				count++
				if base.interaction(true) != 0 {
					return ErrKeystoreUnavailable
				}
				return nil
			}()
			if err != nil {
				return err
			}
		}
	})
	return count, err
}
