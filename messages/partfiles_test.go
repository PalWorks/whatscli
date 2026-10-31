// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
)

func TestIsPartFileName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{".whatscli-1.part", true},
		{".whatscli-1234567890.part", true},
		{".whatscli-.part", false},
		{".whatscli-12a.part", false},
		{".whatscli-12.part.pdf", false},
		{"whatscli-12.part", false},
		{".whatscli-12.PART", false},
		{".whatscli-12.parts", false},
		{"x.whatscli-12.part", false},
		{"report.pdf", false},
		{".whatscli-../x.part", false},
	}
	for _, tt := range tests {
		if got := isPartFileName(tt.name); got != tt.want {
			t.Errorf("isPartFileName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// The temp file downloadMedia creates must be recognised by the sweep.
func TestPartFilePattern_MatchesCreateTemp(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		f, err := os.CreateTemp(dir, partFilePattern)
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
		if name := filepath.Base(f.Name()); !isPartFileName(name) {
			t.Fatalf("CreateTemp name %q not matched", name)
		}
	}
}

func TestSweepPartFiles(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	write := func(p string) {
		t.Helper()
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stale1 := filepath.Join(dir, ".whatscli-123.part")
	stale2 := filepath.Join(dir, ".whatscli-4567.part")
	write(stale1)
	write(stale2)

	// Must survive: non-matching names, a matching symlink (and its target),
	// a matching directory, and a matching file in a subfolder.
	keep := []string{
		filepath.Join(dir, "report.pdf"),
		filepath.Join(dir, ".whatscli-abc.part"),
		filepath.Join(dir, "whatscli-1.part"),
		filepath.Join(dir, ".whatscli-1.part.pdf"),
	}
	for _, p := range keep {
		write(p)
	}
	target := filepath.Join(outside, ".whatscli-9.part")
	write(target)
	link := filepath.Join(dir, ".whatscli-77.part")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	subdir := filepath.Join(dir, ".whatscli-88.part")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(subdir, ".whatscli-5.part")
	write(nested)

	n, err := sweepPartFiles(dir)
	if err != nil || n != 2 {
		t.Fatalf("sweepPartFiles = %d, %v; want 2", n, err)
	}
	for _, p := range []string{stale1, stale2} {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s not removed", p)
		}
	}
	for _, p := range append(keep, target, link, subdir, nested) {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("%s removed: %v", p, err)
		}
	}
}

func TestSweepPartFiles_BlankOrMissingDir(t *testing.T) {
	for _, dir := range []string{"", "  ", filepath.Join(t.TempDir(), "missing")} {
		if n, err := sweepPartFiles(dir); n != 0 || err != nil {
			t.Errorf("sweepPartFiles(%q) = %d, %v", dir, n, err)
		}
	}
}

func TestSweepDownloadFolders_SameFolderOnce(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".whatscli-1.part"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if n := sweepDownloadFolders(dir, dir+string(filepath.Separator), ""); n != 1 {
		t.Errorf("removed %d, want 1", n)
	}
}

// blockingDownloader writes part of a file, then waits for cancellation.
type blockingDownloader struct{ started chan struct{} }

func (b *blockingDownloader) DownloadToFile(ctx context.Context, _ whatsmeow.DownloadableMessage, file whatsmeow.File) error {
	_, _ = file.Write([]byte("partial"))
	close(b.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestShutdown_CancelsDownload(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	dir := t.TempDir()
	msg := storeDoc(t, sm, "C1", "big.pdf")
	dl := &blockingDownloader{started: make(chan struct{})}

	done := make(chan error, 1)
	go func() {
		_, err := sm.fetchMedia(sm.rootContext(), dl, msg, dir)
		done <- err
	}()
	<-dl.started
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || !isPartFileName(entries[0].Name()) {
		t.Fatalf("expected one temp file while downloading, got %v", entries)
	}

	sm.Shutdown()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("download not cancelled by Shutdown")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("leftover files after cancel: %v", entries)
	}
	if sm.rootContext().Err() == nil {
		t.Error("root context still live after Shutdown")
	}
}

func TestShutdown_CancelsBacklogWait(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	ctx := sm.rootContext()
	done := make(chan error, 1)
	go func() {
		_, err := sm.waitForNewMessages(ctx, "c", 0, time.Minute, 10*time.Millisecond)
		done <- err
	}()
	time.Sleep(30 * time.Millisecond)
	// Cancel without closing the DB, so the poll loop sees only the context.
	sm.ctxOnce.Do(sm.initRootContext)
	sm.rootCancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("backlog wait not cancelled")
	}
}

// Shutdown on a SessionManager that never handed out a context must not panic.
func TestShutdown_WithoutRootContext(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	sm.Shutdown()
	if sm.rootContext().Err() == nil {
		t.Error("root context live after Shutdown")
	}
}
