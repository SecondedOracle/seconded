package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	client "github.com/SecondedOracle/seconded/client"
)

const exportWarning = "store this safely; anyone with it can spend this wallet"

var openExportTerminal = exportTerminal
var confirmExportPresence = client.ConfirmOwnerPresence

var (
	errExportTerminal     = errors.New("Key export requires a terminal. Run seconded-mcp export-key yourself in your own terminal.")
	errExportConfirmation = errors.New("Key export cancelled: type the wallet's last 6 address characters in your own terminal and press Enter to confirm.")
	errExportWrite        = errors.New("Key export could not write to your terminal.")
)

type exportConsole struct {
	input  *os.File
	output *os.File
}

func (c *exportConsole) close() {
	c.input.Close()
	c.output.Close()
}

// Open the controlling terminal before touching a profile, even an uninitialized one.
func exportKey(dir, exe string) error {
	terminal, err := openExportTerminal()
	if err != nil {
		return errExportTerminal
	}
	defer terminal.close()
	g, err := client.InstalledEngine(dir, exe)
	if err != nil {
		return err
	}
	return exportToTerminal(g, terminal)
}

// Export is CLI-only. Neither the confirmation nor key uses captured process streams.
func exportToTerminal(g *client.Engine, terminal *exportConsole) error {
	unlock, err := g.Files.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	v, err := client.LoadVault(g.Store)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(terminal.output, "%s\nWallet: %s\nType the last 6 address characters and press Enter to export: ", exportWarning, v.Address); err != nil {
		return errExportWrite
	}
	// Bound input and require Enter: an EOF after six characters is not confirmation.
	confirmation, err := bufio.NewReader(io.LimitReader(terminal.input, 8)).ReadString('\n')
	confirmation = strings.TrimSuffix(strings.TrimSuffix(confirmation, "\n"), "\r")
	if err != nil || !strings.EqualFold(confirmation, v.Address[len(v.Address)-6:]) {
		return errExportConfirmation
	}
	if err = confirmExportPresence("Export private key for wallet "+v.Address+". Profile: "+g.Files.Dir, terminal.input, terminal.output); err != nil {
		return errExportConfirmation
	}
	if _, err = fmt.Fprintln(terminal.output, v.Key); err != nil {
		return errExportWrite
	}
	return nil
}
