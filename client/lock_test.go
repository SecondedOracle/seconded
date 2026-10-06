package client

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestLockChild(t *testing.T) {
	if os.Getenv("SECONDED_LOCK_CHILD") != "1" {
		return
	}
	files, e := OpenFiles(os.Getenv("SECONDED_LOCK_DIR"))
	if e != nil {
		os.Exit(2)
	}
	fmt.Println("ready")
	unlock, e := files.Lock()
	if e != nil {
		os.Exit(2)
	}
	fmt.Println("acquired")
	unlock()
	os.Exit(0)
}
func TestFileLockAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	files, e := OpenFiles(dir)
	if e != nil {
		t.Fatal(e)
	}
	unlock, e := files.Lock()
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLockChild$")
	cmd.Env = append(os.Environ(), "SECONDED_LOCK_CHILD=1", "SECONDED_LOCK_DIR="+dir)
	out, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	scanner := bufio.NewScanner(out)
	if !scanner.Scan() || scanner.Text() != "ready" {
		t.Fatal("child start")
	}
	got := make(chan string, 1)
	go func() {
		if scanner.Scan() {
			got <- scanner.Text()
		} else {
			got <- "closed"
		}
	}()
	select {
	case s := <-got:
		t.Fatal("second process bypassed lock", s)
	case <-time.After(100 * time.Millisecond):
	}
	unlock()
	unlock = nil
	select {
	case s := <-got:
		if s != "acquired" {
			t.Fatal(s)
		}
	case <-ctx.Done():
		t.Fatal("lock never released")
	}
	if e = cmd.Wait(); e != nil {
		t.Fatal(e)
	}
}
