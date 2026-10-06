//go:build darwin || linux

package client

func systemSSHKeygen() string { return "/usr/bin/ssh-keygen" }
