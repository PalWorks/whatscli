// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Schema as shipped in v1.5.0: media columns present, no local_path.
func TestMigration_AddsLocalPath(t *testing.T) {
	db := newSingleConnDB(t)
	_, err := db.Exec(`
	CREATE TABLE conversations (jid TEXT PRIMARY KEY, name TEXT, last_msg_time INTEGER, preview TEXT, unread INTEGER, is_pinned BOOLEAN, is_archived BOOLEAN DEFAULT 0);
	CREATE TABLE messages (id TEXT PRIMARY KEY, chat_id TEXT, contact_id TEXT, contact_name TEXT, contact_short TEXT,
		timestamp INTEGER, from_me BOOLEAN, forwarded BOOLEAN, text TEXT,
		media_kind TEXT NOT NULL DEFAULT '', mime_type TEXT NOT NULL DEFAULT '', file_name TEXT NOT NULL DEFAULT '', media_proto BLOB);
	INSERT INTO messages (id, chat_id, contact_id, contact_name, contact_short, timestamp, from_me, forwarded, text)
		VALUES ('OLD1', 'a@s.whatsapp.net', 'a@s.whatsapp.net', 'Alice', 'Alice', 10, 0, 0, 'hello');
	`)
	if err != nil {
		t.Fatalf("seed v1.5.0 schema: %v", err)
	}
	md := &MessageDatabase{}
	for i := 0; i < 3; i++ {
		if err := md.InitWithDB(db); err != nil {
			t.Fatalf("InitWithDB run %d: %v", i+1, err)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name = 'local_path'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("local_path columns = %d err=%v", n, err)
	}
	msg, found, err := md.GetMessage("OLD1")
	if err != nil || !found || msg.Text != "hello" || msg.LocalPath != "" {
		t.Fatalf("old row after migration: %+v found=%v err=%v", msg, found, err)
	}
	if ok, err := md.SetLocalPath("a@s.whatsapp.net", "OLD1", "/x/y.pdf"); err != nil || !ok {
		t.Fatalf("SetLocalPath: ok=%v err=%v", ok, err)
	}
	if msg, _, _ := md.GetMessage("OLD1"); msg.LocalPath != "/x/y.pdf" {
		t.Errorf("LocalPath = %q", msg.LocalPath)
	}
}

func TestSetLocalPath_ScopedAndClearedByRevoke(t *testing.T) {
	md := newTestDB(t)
	if _, err := md.AddMessage(Message{Id: "P1", ChatId: "c", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := md.SetLocalPath("other", "P1", "/a"); ok {
		t.Error("SetLocalPath crossed chats")
	}
	if ok, _ := md.SetLocalPath("c", "P1", "/a"); !ok {
		t.Fatal("SetLocalPath did not update")
	}
	if _, err := md.MarkMessageRevoked("c", "P1"); err != nil {
		t.Fatal(err)
	}
	if msg, _, _ := md.GetMessage("P1"); msg.LocalPath != "" {
		t.Errorf("revoke kept local path %q", msg.LocalPath)
	}
	if ok, _ := md.SetLocalPath("c", "P1", "/a"); ok {
		t.Error("revoked message got a local path")
	}
}

// storeDoc stores a document message and returns it as loaded from SQLite.
func storeDoc(t *testing.T, sm *SessionManager, id, name string) Message {
	t.Helper()
	if _, err := sm.db.AddMessage(docMsg(t, id, name)); err != nil {
		t.Fatal(err)
	}
	msg, found, err := sm.db.GetMessage(id)
	if err != nil || !found {
		t.Fatalf("GetMessage %s: found=%v err=%v", id, found, err)
	}
	return msg
}

func reload(t *testing.T, sm *SessionManager, id string) Message {
	t.Helper()
	msg, _, err := sm.db.GetMessage(id)
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestFetchMedia_RepeatDownloadReusesFile(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	dir := t.TempDir()
	dl := &fakeDownloader{data: []byte("doc")}
	msg := storeDoc(t, sm, "D1", "report.pdf")

	first, err := sm.fetchMedia(context.Background(), dl, msg, dir)
	if err != nil {
		t.Fatalf("first download: %v", err)
	}
	if filepath.Base(first) != "report.pdf" {
		t.Errorf("first path %q", first)
	}
	if got := reload(t, sm, "D1").LocalPath; got != first {
		t.Errorf("recorded %q, want %q", got, first)
	}

	// Second and third download: no network call, no second copy.
	for i := 0; i < 2; i++ {
		again, err := sm.fetchMedia(context.Background(), dl, reload(t, sm, "D1"), dir)
		if err != nil {
			t.Fatalf("repeat %d: %v", i+1, err)
		}
		resolvedFirst, _ := filepath.EvalSymlinks(first)
		if again != resolvedFirst {
			t.Errorf("repeat %d path %q, want %q", i+1, again, resolvedFirst)
		}
	}
	if dl.calls != 1 {
		t.Errorf("downloader called %d times, want 1", dl.calls)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected one file, found %d", len(entries))
	}
}

func TestFetchMedia_FallbackWhenRecordedPathUnusable(t *testing.T) {
	tests := []struct {
		name string
		// setup returns the path to record, given the download dir and a
		// directory outside it.
		setup func(t *testing.T, dir, outside string) string
		// want is the expected base name of the new download.
		want string
	}{
		{"file deleted", func(t *testing.T, dir, _ string) string {
			return filepath.Join(dir, "report.pdf") // never created
		}, "report.pdf"},
		{"outside download folder", func(t *testing.T, _, outside string) string {
			p := filepath.Join(outside, "report.pdf")
			mustWrite(t, p)
			return p
		}, "report.pdf"},
		{"symlink inside folder", func(t *testing.T, dir, outside string) string {
			target := filepath.Join(outside, "secret.txt")
			mustWrite(t, target)
			link := filepath.Join(dir, "link.pdf")
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("symlink: %v", err)
			}
			return link
		}, "report.pdf"},
		{"subfolder of download folder", func(t *testing.T, dir, _ string) string {
			p := filepath.Join(dir, "sub", "report.pdf")
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			mustWrite(t, p)
			return p
		}, "report.pdf"},
		{"relative path", func(t *testing.T, _, _ string) string {
			return "report.pdf"
		}, "report.pdf"},
		{"directory", func(t *testing.T, dir, _ string) string {
			p := filepath.Join(dir, "adir")
			if err := os.Mkdir(p, 0o755); err != nil {
				t.Fatal(err)
			}
			return p
		}, "report.pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, _ := newTestSessionManager(t)
			dir, outside := t.TempDir(), t.TempDir()
			msg := storeDoc(t, sm, "D2", "report.pdf")
			recorded := tt.setup(t, dir, outside)
			if _, err := sm.db.SetLocalPath(msg.ChatId, msg.Id, recorded); err != nil {
				t.Fatal(err)
			}
			dl := &fakeDownloader{data: []byte("fresh")}
			got, err := sm.fetchMedia(context.Background(), dl, reload(t, sm, "D2"), dir)
			if err != nil {
				t.Fatalf("fetchMedia: %v", err)
			}
			if dl.calls != 1 {
				t.Errorf("expected a new download, calls=%d", dl.calls)
			}
			if filepath.Dir(got) != dir || filepath.Base(got) != tt.want {
				t.Errorf("path %q, want %s in %s", got, tt.want, dir)
			}
			if b, _ := os.ReadFile(got); string(b) != "fresh" {
				t.Errorf("content %q", b)
			}
			if rec := reload(t, sm, "D2").LocalPath; rec != got {
				t.Errorf("recorded %q, want %q", rec, got)
			}
		})
	}
}

func mustWrite(t *testing.T, p string) {
	t.Helper()
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
}
