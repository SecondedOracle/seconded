package client

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

// TerminalSwitchWallet is deliberately absent from the MCP dispatch table.
// Approval comes from the controlling terminal, never tool arguments or stdin.
func (g *Engine) TerminalSwitchWallet(earlier bool) (any, error) {
	unlock, err := g.Files.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	v, err := LoadVault(g.Store)
	if err != nil {
		return nil, err
	}
	i, err := readInstallation(g.Files)
	if err != nil {
		return nil, err
	}
	destination := i.NewerAddress
	if earlier {
		destination = i.EarlierAddress
	}
	if !addressPattern.MatchString(destination) {
		return nil, ErrInvalid
	}
	name := "/dev/tty"
	if runtime.GOOS == "windows" {
		name = "CONIN$"
	}
	tty, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return nil, ErrTerminalPolicyRequired
	}
	defer tty.Close()
	info, err := tty.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return nil, ErrTerminalPolicyRequired
	}
	out := tty
	if runtime.GOOS == "windows" {
		out, err = os.OpenFile("CONOUT$", os.O_WRONLY, 0)
		if err != nil {
			return nil, ErrTerminalPolicyRequired
		}
		defer out.Close()
	}
	if _, err = fmt.Fprintf(out, "Switch wallet from %s to %s. Type switch %s to confirm: ", v.Address, destination, v.Address[len(v.Address)-6:]); err != nil {
		return nil, err
	}
	answer, err := bufio.NewReader(io.LimitReader(tty, 64)).ReadString('\n')
	if err != nil || strings.TrimSpace(answer) != "switch "+v.Address[len(v.Address)-6:] {
		return nil, ErrTerminalPolicyRequired
	}
	if err = ConfirmOwnerPresence(fmt.Sprintf("Switch wallet from %s to %s. Profile: %s", v.Address, destination, g.Files.Dir), tty, out); err != nil {
		return nil, ErrTerminalPolicyRequired
	}
	return g.switchWalletLocked(earlier)
}
