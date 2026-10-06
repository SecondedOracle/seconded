//go:build !darwin

package client

import "github.com/zalando/go-keyring"

func platformKeychainGet(path, service, account string) (string, error) {
	if path != "" {
		return "", ErrKeystoreUnavailable
	}
	return keyring.Get(service, account)
}

func platformKeychainSet(path, service, account, value string) error {
	if path != "" {
		return ErrKeystoreUnavailable
	}
	return keyring.Set(service, account, value)
}
