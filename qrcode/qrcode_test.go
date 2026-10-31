package qrcode

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/skip2/go-qrcode"
)

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

// TestRenderKeepsFullQuietZone checks that every bitmap module, including the
// 4-module ISO quiet zone that go-qrcode adds, is rendered (upstream #138).
func TestRenderKeepsFullQuietZone(t *testing.T) {
	const content = "2@abcdefghijklmnopqrstuvwxyz0123456789,ABCDEFG,HIJKLMNOP"
	qr, err := qrcode.New(content, qrcode.Low)
	if err != nil {
		t.Fatal(err)
	}
	bmp := qr.Bitmap()
	for r := 0; r < 4; r++ {
		for _, dark := range bmp[r] {
			if dark {
				t.Fatalf("bitmap row %d is not part of a light quiet zone", r)
			}
		}
	}

	out := New().Get(content)
	if out == nil {
		t.Fatal("Get returned nil")
	}
	lines := strings.Split(strings.TrimRight(string(*out), "\n"), "\n")
	if want := (len(bmp) + 1) / 2; len(lines) != want {
		t.Fatalf("rendered %d lines, want %d (half-block rows for %d modules)", len(lines), want, len(bmp))
	}
	for i, line := range lines {
		plain := ansi.ReplaceAllString(line, "")
		if n := utf8.RuneCountInString(plain); n != len(bmp[0]) {
			t.Fatalf("line %d has %d columns, want %d", i, n, len(bmp[0]))
		}
	}
	// First two rendered lines (4 modules) and the first/last 4 columns of
	// every line must be blank (light).
	for i := 0; i < 2; i++ {
		if strings.TrimSpace(ansi.ReplaceAllString(lines[i], "")) != "" {
			t.Errorf("quiet zone line %d contains dark modules", i)
		}
	}
	for i, line := range lines {
		runes := []rune(ansi.ReplaceAllString(line, ""))
		edge := string(runes[:4]) + string(runes[len(runes)-4:])
		if strings.TrimSpace(edge) != "" {
			t.Errorf("line %d: side quiet zone contains dark modules: %q", i, edge)
		}
	}
}
