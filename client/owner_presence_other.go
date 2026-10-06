//go:build !darwin

package client

func platformOwnerPresence(string) error { return errPresenceUnavailable }
