// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// openerCommand builds the system opener for target without a shell:
// xdg-open on Linux and other Unixes, open on macOS, rundll32
// url.dll,FileProtocolHandler on Windows (same commands as open-golang).
func openerCommand(target string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target)
	case "windows":
		rundll32 := filepath.Join(os.Getenv("SYSTEMROOT"), "System32", "rundll32.exe")
		return exec.Command(rundll32, "url.dll,FileProtocolHandler", target)
	default:
		return exec.Command("xdg-open", target)
	}
}

// startOpener starts cmd without waiting for it, so a slow or hanging opener
// never blocks the caller (ROADMAP row 5). A start failure is returned. The
// process is reaped in the background and a non-zero exit is passed to
// onExit, which runs on that background goroutine.
func startOpener(cmd *exec.Cmd, onExit func(error)) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", filepath.Base(cmd.Path), err)
	}
	go func() {
		if err := cmd.Wait(); err != nil && onExit != nil {
			onExit(fmt.Errorf("%s: %w", filepath.Base(cmd.Path), err))
		}
	}()
	return nil
}
