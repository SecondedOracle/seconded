//go:build darwin || linux

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func exportTerminal() (*exportConsole, error) {
	input, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		return nil, errExportTerminal
	}
	output, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		input.Close()
		return nil, errExportTerminal
	}
	console := &exportConsole{input: input, output: output}
	for _, file := range []*os.File{input, output} {
		if _, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ); err != nil {
			console.close()
			return nil, errExportTerminal
		}
	}
	return console, nil
}
