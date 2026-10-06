//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func exportTerminal() (*exportConsole, error) {
	// Console devices stay separate from inherited stdin, stdout and stderr.
	// GetConsoleMode needs read access even on the output handle.
	input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil, errExportTerminal
	}
	output, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		input.Close()
		return nil, errExportTerminal
	}
	console := &exportConsole{input: input, output: output}
	for _, file := range []*os.File{input, output} {
		var mode uint32
		if err := windows.GetConsoleMode(windows.Handle(file.Fd()), &mode); err != nil {
			console.close()
			return nil, errExportTerminal
		}
	}
	// Never attach or allocate a console on behalf of an unattended caller.
	// The existing console must provide both the confirmation and key output.
	return console, nil
}
