// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// writeViewer writes an executable shell script used as show_command.
func writeViewer(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "viewer.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// imageFixture creates a preview directory with one image file in it.
func imageFixture(t *testing.T) (previewDir, image, toolsDir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the viewer")
	}
	previewDir = t.TempDir()
	toolsDir = t.TempDir()
	image = filepath.Join(previewDir, "photo.jpg")
	if err := os.WriteFile(image, []byte("IMAGEDATA"), 0o600); err != nil {
		t.Fatal(err)
	}
	return previewDir, image, toolsDir
}

func TestRenderImage_ReturnsViewerOutput(t *testing.T) {
	preview, image, tools := imageFixture(t)
	viewer := writeViewer(t, tools, `printf 'ART:'; cat "$2"`)

	out, err := renderImage(viewer+" --color", image, 5*time.Second, "", preview)
	if err != nil {
		t.Fatalf("renderImage: %v", err)
	}
	if string(out) != "ART:IMAGEDATA" {
		t.Errorf("output = %q", out)
	}
}

func TestRenderImage_SlowViewerTimesOut(t *testing.T) {
	preview, image, tools := imageFixture(t)
	viewer := writeViewer(t, tools, "exec sleep 30")

	begin := time.Now()
	_, err := renderImage(viewer, image, 200*time.Millisecond, preview)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if d := time.Since(begin); d > 5*time.Second {
		t.Errorf("viewer was not stopped at the timeout: took %v", d)
	}
}

func TestRenderImage_ChildHoldingStdoutDoesNotHang(t *testing.T) {
	preview, image, tools := imageFixture(t)
	// The script's background child keeps stdout open after a kill.
	viewer := writeViewer(t, tools, "sleep 3 &\nwait")

	begin := time.Now()
	_, err := renderImage(viewer, image, 200*time.Millisecond, preview)
	if err == nil {
		t.Fatal("expected an error")
	}
	if d := time.Since(begin); d > 5*time.Second {
		t.Errorf("renderImage hung on an inherited stdout: took %v", d)
	}
}

func TestRenderImage_ViewerFailureReported(t *testing.T) {
	preview, image, tools := imageFixture(t)
	viewer := writeViewer(t, tools, "echo 'cannot decode' >&2; exit 3")

	_, err := renderImage(viewer, image, 5*time.Second, preview)
	if err == nil || !strings.Contains(err.Error(), "cannot decode") {
		t.Fatalf("err = %v, want the viewer's stderr", err)
	}
}

func TestRenderImage_Refusals(t *testing.T) {
	preview, image, tools := imageFixture(t)
	marker := filepath.Join(tools, "ran")
	viewer := writeViewer(t, tools, "touch "+marker)
	outside := filepath.Join(tools, "outside.jpg")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, cmd, path string
		dirs            []string
		want            string
	}{
		{"missing file", viewer, filepath.Join(preview, "nope.jpg"), []string{preview}, "not found"},
		{"outside the preview directory", viewer, outside, []string{preview}, ""},
		{"no configured directory", viewer, image, []string{""}, ""},
		{"empty show_command", "  ", image, []string{preview}, "show_command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := renderImage(tt.cmd, tt.path, 5*time.Second, tt.dirs...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want one containing %q", err, tt.want)
			}
			if _, statErr := os.Stat(marker); statErr == nil {
				t.Fatal("viewer ran for a refused file")
			}
		})
	}
}
