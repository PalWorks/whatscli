// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStartOpener_StartFailureReturned(t *testing.T) {
	cmd := exec.Command("whatscli-no-such-opener-binary", "https://example.org")
	err := startOpener(cmd, func(error) { t.Error("onExit must not run when start fails") })
	if err == nil || !strings.Contains(err.Error(), "start") {
		t.Fatalf("err = %v, want a start error", err)
	}
}

func TestStartOpener_DoesNotBlockAndReaps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix sleep")
	}
	cmd := exec.Command("sleep", "5")
	exited := make(chan error, 1)
	begin := time.Now()
	if err := startOpener(cmd, func(err error) { exited <- err }); err != nil {
		t.Fatalf("start: %v", err)
	}
	if d := time.Since(begin); d > time.Second {
		t.Fatalf("startOpener blocked for %v", d)
	}
	// Killing the opener makes Wait return; the background goroutine reaps
	// the process and reports the abnormal exit.
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	select {
	case err := <-exited:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("onExit err = %v, want an exit error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("onExit not called after the opener exited")
	}
}

func TestStartOpener_CleanExitSilent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix true")
	}
	called := make(chan struct{}, 1)
	if err := startOpener(exec.Command("true"), func(error) { called <- struct{}{} }); err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-called:
		t.Fatal("onExit called for a clean exit")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestOpenerCommand_TargetIsOneArgument(t *testing.T) {
	target := "https://example.org/a b;echo x"
	cmd := openerCommand(target)
	if got := cmd.Args[len(cmd.Args)-1]; got != target {
		t.Errorf("target must be passed as one argument, got %q", got)
	}
	for _, a := range cmd.Args {
		if a == "-c" || a == "/C" {
			t.Errorf("opener must not run through a shell: %v", cmd.Args)
		}
	}
}
