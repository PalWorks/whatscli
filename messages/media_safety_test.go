package messages

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSanitizeFileName_WindowsHardening(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"CON", "_CON", true},
		{"con.txt", "_con.txt", true},
		{"Nul.tar.gz", "_Nul.tar.gz", true},
		{"PRN", "_PRN", true},
		{"aux.pdf", "_aux.pdf", true},
		{"COM1", "_COM1", true},
		{"com9.exe", "_com9.exe", true},
		{"LPT1.txt", "_LPT1.txt", true},
		{"lpt0", "_lpt0", true},
		{"COM¹.txt", "_COM¹.txt", true},
		{"CONIN$", "_CONIN$", true},
		{"CON .txt", "_CON .txt", true},
		{"CONSOLE.txt", "CONSOLE.txt", true},
		{"COM10.txt", "COM10.txt", true},
		{"icon.png", "icon.png", true},
		{"report.pdf:evil.exe", "report.pdf_evil.exe", true},
		{"C:evil.txt", "C_evil.txt", true},
		{`a<b>c"d|e?f*g.txt`, "a_b_c_d_e_f_g.txt", true},
		{"tab\there.txt", "tab_here.txt", true},
		{"bell\x07.txt", "bell_.txt", true},
		{"invoice.pdf.", "invoice.pdf", true},
		{"invoice.exe . .", "invoice.exe", true},
		{"...", "", false},
		{". .", "", false},
		{"  leading.txt", "leading.txt", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := sanitizeFileName(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Errorf("sanitizeFileName(%q) = %q,%v want %q,%v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestSafeToOpen(t *testing.T) {
	allowed := []string{"a.jpg", "a.JPEG", "a.png", "a.webp", "a.mp4", "a.mov", "a.ogg", "a.opus", "a.mp3", "a.m4a", "a.pdf", "a.PDF", "a.txt"}
	refused := []string{"a.exe", "a.EXE", "a.bat", "a.cmd", "a.com", "a.scr", "a.pif", "a.lnk", "a.url", "a.js", "a.jse",
		"a.vbs", "a.wsf", "a.hta", "a.msi", "a.ps1", "a.reg", "a.cpl", "a.jar", "a.desktop", "a.sh", "a.AppImage",
		"a.html", "a.svg", "a.docm", "a.docx", "a.xlsm", "a.zip", "a.iso", "noext", "a.pdf.exe", "a.txt.lnk"}
	for _, p := range allowed {
		if !safeToOpen(filepath.Join("dir", p)) {
			t.Errorf("safeToOpen(%q) = false, want true", p)
		}
	}
	for _, p := range refused {
		if safeToOpen(filepath.Join("dir", p)) {
			t.Errorf("safeToOpen(%q) = true, want false", p)
		}
	}
}

func TestDeliverDownload_OpenGate(t *testing.T) {
	tests := []struct {
		path   string
		opened bool
	}{
		{"/tmp/x/photo.jpg", true},
		{"/tmp/x/doc.pdf", true},
		{"/tmp/x/Invoice.exe", false},
		{"/tmp/x/run.bat", false},
		{"/tmp/x/link.lnk", false},
		{"/tmp/x/noextension", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			sm, mock := newTestSessionManager(t)
			sm.deliverDownload(modeOpen, tt.path)
			opened := len(mock.Files) == 1 && mock.Files[0] == tt.path
			if opened != tt.opened {
				t.Fatalf("opened=%v want %v (files=%v texts=%v)", opened, tt.opened, mock.Files, mock.Texts)
			}
			if !tt.opened {
				joined := strings.Join(mock.Texts, "\n")
				if !strings.Contains(joined, tt.path) || !strings.Contains(joined, "not opened") {
					t.Errorf("refusal must print path and reason, got %q", joined)
				}
			}
		})
	}
}

func TestDeliverDownload_DownloadAndShowUnchanged(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.deliverDownload(modeDownload, "/tmp/x/Invoice.exe")
	if len(mock.Files) != 0 || len(mock.Texts) != 1 || !strings.Contains(mock.Texts[0], "Invoice.exe") {
		t.Errorf("download must only print the path: files=%v texts=%v", mock.Files, mock.Texts)
	}
	sm.deliverDownload(modeShow, "/tmp/x/photo.jpg")
	if len(mock.Files) != 1 {
		t.Errorf("show must pass the file to PrintFile: %v", mock.Files)
	}
}

func TestResolveFileWithin(t *testing.T) {
	root := t.TempDir()
	// Resolve the temp root itself (macOS /var -> /private/var).
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	mk := func(rel string) string {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	inDL := mk("home/u/Downloads/a.jpg")
	inSibling := mk("home/u/Downloads2/b.jpg")
	inPreview := mk("home/u/preview/c.jpg")
	outside := mk("home/u/secret.jpg")
	dl := filepath.Join(root, "home/u/Downloads")
	preview := filepath.Join(root, "home/u/preview")

	t.Run("inside download dir", func(t *testing.T) {
		got, err := ResolveFileWithin(inDL, dl, preview)
		if err != nil || got != inDL {
			t.Errorf("got %q err=%v", got, err)
		}
	})
	t.Run("inside preview dir", func(t *testing.T) {
		if _, err := ResolveFileWithin(inPreview, dl, preview); err != nil {
			t.Errorf("err=%v", err)
		}
	})
	t.Run("sibling Downloads2 rejected", func(t *testing.T) {
		if _, err := ResolveFileWithin(inSibling, dl, preview); !errors.Is(err, ErrOutsideAllowedDirs) {
			t.Errorf("err=%v, want ErrOutsideAllowedDirs", err)
		}
	})
	t.Run("parent dir rejected", func(t *testing.T) {
		if _, err := ResolveFileWithin(outside, dl, preview); !errors.Is(err, ErrOutsideAllowedDirs) {
			t.Errorf("err=%v", err)
		}
	})
	t.Run("traversal in file path rejected", func(t *testing.T) {
		if _, err := ResolveFileWithin(filepath.Join(dl, "..", "secret.jpg"), dl); !errors.Is(err, ErrOutsideAllowedDirs) {
			t.Errorf("err=%v", err)
		}
	})
	t.Run("the dir itself rejected", func(t *testing.T) {
		if _, err := ResolveFileWithin(dl, dl); !errors.Is(err, ErrOutsideAllowedDirs) {
			t.Errorf("err=%v", err)
		}
	})
	t.Run("empty dirs never mean cwd", func(t *testing.T) {
		if _, err := ResolveFileWithin(inDL, "", "  "); !errors.Is(err, ErrOutsideAllowedDirs) {
			t.Errorf("err=%v", err)
		}
	})
	t.Run("missing file", func(t *testing.T) {
		if _, err := ResolveFileWithin(filepath.Join(dl, "nope.jpg"), dl); err == nil || errors.Is(err, ErrOutsideAllowedDirs) {
			t.Errorf("err=%v, want resolve error", err)
		}
	})
	t.Run("non-existent configured dir", func(t *testing.T) {
		if _, err := ResolveFileWithin(inDL, filepath.Join(root, "does/not/exist")); !errors.Is(err, ErrOutsideAllowedDirs) {
			t.Errorf("err=%v", err)
		}
	})

	if runtime.GOOS == "windows" {
		t.Skip("symlink cases need privileges on Windows")
	}
	// Symlinked home: config says <root>/linkhome/u/Downloads, real path is <root>/home/u/Downloads.
	if err := os.Symlink(filepath.Join(root, "home"), filepath.Join(root, "linkhome")); err != nil {
		t.Fatal(err)
	}
	linkDL := filepath.Join(root, "linkhome/u/Downloads")
	t.Run("symlinked home, configured via link", func(t *testing.T) {
		if _, err := ResolveFileWithin(filepath.Join(linkDL, "a.jpg"), linkDL); err != nil {
			t.Errorf("err=%v", err)
		}
	})
	t.Run("symlinked home, file via real path", func(t *testing.T) {
		if _, err := ResolveFileWithin(inDL, linkDL); err != nil {
			t.Errorf("err=%v", err)
		}
	})
	t.Run("symlink inside download dir pointing out is rejected", func(t *testing.T) {
		link := filepath.Join(dl, "escape.jpg")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		if _, err := ResolveFileWithin(link, dl); !errors.Is(err, ErrOutsideAllowedDirs) {
			t.Errorf("err=%v", err)
		}
	})
	t.Run("non-existent dir under symlinked parent", func(t *testing.T) {
		// <root>/linkhome/u/newdir does not exist; it must resolve to <root>/home/u/newdir.
		got, err := resolveDir(filepath.Join(root, "linkhome/u/newdir/sub"))
		if err != nil || got != filepath.Join(root, "home/u/newdir/sub") {
			t.Errorf("resolveDir = %q err=%v", got, err)
		}
	})
}
