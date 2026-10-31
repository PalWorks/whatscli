// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// newSingleConnDB opens an in-memory SQLite pinned to one connection, so
// goroutines in a test always see the same database.
func newSingleConnDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigration_OldSchemaDB(t *testing.T) {
	db := newSingleConnDB(t)
	// Schema as shipped before Phase 2a (no media columns, no is_archived).
	_, err := db.Exec(`
	CREATE TABLE conversations (jid TEXT PRIMARY KEY, name TEXT, last_msg_time INTEGER, preview TEXT, unread INTEGER, is_pinned BOOLEAN);
	CREATE TABLE messages (id TEXT PRIMARY KEY, chat_id TEXT, contact_id TEXT, contact_name TEXT, contact_short TEXT,
		timestamp INTEGER, from_me BOOLEAN, forwarded BOOLEAN, text TEXT);
	INSERT INTO conversations VALUES ('a@s.whatsapp.net', 'Alice', 10, 'hi', 0, 0);
	INSERT INTO messages VALUES ('OLD1', 'a@s.whatsapp.net', 'a@s.whatsapp.net', 'Alice', 'Alice', 10, 0, 0, 'hello');
	`)
	if err != nil {
		t.Fatalf("seed old schema: %v", err)
	}

	md := &MessageDatabase{}
	// Run twice: the migration must be idempotent.
	for i := 0; i < 2; i++ {
		if err := md.InitWithDB(db); err != nil {
			t.Fatalf("InitWithDB run %d: %v", i+1, err)
		}
	}

	msg, found, err := md.GetMessage("OLD1")
	if err != nil || !found {
		t.Fatalf("GetMessage OLD1: found=%v err=%v", found, err)
	}
	if msg.Text != "hello" || msg.MediaKind != "" || msg.MediaProto != nil {
		t.Errorf("old row not preserved with empty media: %+v", msg)
	}
	if _, err := downloadableFromMessage(msg); !errors.Is(err, ErrNotDownloadable) {
		t.Errorf("old row should not be downloadable, got %v", err)
	}
	convs, err := md.GetConversations()
	if err != nil || len(convs) != 1 || convs[0].IsArchived {
		t.Errorf("conversations after migration: %+v err=%v", convs, err)
	}
	for _, col := range []string{"media_kind", "mime_type", "file_name", "media_proto", "local_path"} {
		if _, err := db.Exec("SELECT " + col + " FROM messages LIMIT 1"); err != nil {
			t.Errorf("column %s missing: %v", col, err)
		}
	}
}

func TestAddColumnIfMissing_PropagatesOtherErrors(t *testing.T) {
	md := &MessageDatabase{db: newSingleConnDB(t)}
	err := md.addColumnIfMissing("no_such_table", "x", "TEXT")
	if err == nil {
		t.Fatal("expected error for missing table, got nil")
	}
}

func sampleDocMessage() *waE2E.Message {
	return &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
		URL:           proto.String("https://mmg.whatsapp.net/x"),
		DirectPath:    proto.String("/v/t62/abc"),
		MediaKey:      []byte{1, 2, 3, 4},
		FileEncSHA256: []byte{5, 6},
		FileSHA256:    []byte{7, 8},
		FileLength:    proto.Uint64(1234),
		Mimetype:      proto.String("application/pdf"),
		FileName:      proto.String("report.pdf"),
		JPEGThumbnail: bytes.Repeat([]byte{0xff}, 2048),
	}}
}

func TestMediaMetadata_RoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		wa       *waE2E.Message
		kind     string
		mime     string
		fileName string
	}{
		{"document", sampleDocMessage(), MediaKindDocument, "application/pdf", "report.pdf"},
		{"image", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			DirectPath: proto.String("/img"), MediaKey: []byte{9}, Mimetype: proto.String("image/jpeg"),
			JPEGThumbnail: []byte{1, 2, 3},
		}}, MediaKindImage, "image/jpeg", ""},
		{"video", &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
			DirectPath: proto.String("/vid"), MediaKey: []byte{9}, Mimetype: proto.String("video/mp4"),
		}}, MediaKindVideo, "video/mp4", ""},
		{"audio", &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
			DirectPath: proto.String("/aud"), MediaKey: []byte{9}, Mimetype: proto.String("audio/ogg; codecs=opus"),
		}}, MediaKindAudio, "audio/ogg; codecs=opus", ""},
		{"sticker", &waE2E.Message{StickerMessage: &waE2E.StickerMessage{
			DirectPath: proto.String("/stk"), MediaKey: []byte{9}, Mimetype: proto.String("image/webp"),
		}}, MediaKindSticker, "image/webp", ""},
	}
	md := newTestDB(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := Message{Id: "ID-" + tt.name, ChatId: "c@s.whatsapp.net", Text: "[x]", Timestamp: 1}
			if err := attachMediaInfo(&msg, tt.wa); err != nil {
				t.Fatalf("attachMediaInfo: %v", err)
			}
			addTestMsg(t, md, msg)
			got, found, err := md.GetMessage(msg.Id)
			if err != nil || !found {
				t.Fatalf("GetMessage: found=%v err=%v", found, err)
			}
			if got.MediaKind != tt.kind || got.MimeType != tt.mime || got.FileName != tt.fileName {
				t.Errorf("got kind=%q mime=%q file=%q", got.MediaKind, got.MimeType, got.FileName)
			}
			d, err := downloadableFromMessage(got)
			if err != nil {
				t.Fatalf("downloadableFromMessage: %v", err)
			}
			if d.GetDirectPath() == "" || len(d.GetMediaKey()) == 0 {
				t.Errorf("download fields lost: %+v", d)
			}
			// Thumbnails are stripped from the stored proto.
			var wa waE2E.Message
			if err := proto.Unmarshal(got.MediaProto, &wa); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(wa.GetImageMessage().GetJPEGThumbnail()) != 0 || len(wa.GetDocumentMessage().GetJPEGThumbnail()) != 0 {
				t.Error("thumbnail was not stripped")
			}
		})
	}
}

func TestExtractMediaInfo_TextMessage(t *testing.T) {
	kind, _, _, raw, err := extractMediaInfo(&waE2E.Message{Conversation: proto.String("hi")})
	if kind != "" || raw != nil || err != nil {
		t.Errorf("text message produced media info: kind=%q raw=%v err=%v", kind, raw, err)
	}
}

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"report.pdf", "report.pdf", true},
		{"../../etc/passwd", "passwd", true},
		{`..\..\Windows\evil.exe`, "evil.exe", true},
		{"/abs/path/file.txt", "file.txt", true},
		{"dir/", "dir", true},
		{"", "", false},
		{".", "", false},
		{"..", "", false},
		{"../", "", false},
		{`..\`, "", false},
		{"/", "", false},
		{"a\x00b.txt", "ab.txt", true},
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

func TestDownloadFileName(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want string
	}{
		{"uses file name", Message{Id: "ABC", FileName: "x.pdf"}, "x.pdf"},
		{"strips traversal", Message{Id: "ABC", FileName: "../../x.pdf"}, "x.pdf"},
		{"dotdot falls back to id", Message{Id: "ABC", FileName: "..", MimeType: "application/pdf"}, "ABC.pdf"},
		{"no ext for unknown mime", Message{Id: "ABC", MimeType: "x-unknown/zzz"}, "ABC"},
		{"mime params ignored", Message{Id: "ABC", MimeType: "image/png; foo=bar"}, "ABC.png"},
		{"hostile id", Message{Id: "../../evil", MimeType: ""}, "evil"},
		{"jpeg is .jpg", Message{Id: "ABC", MimeType: "image/jpeg"}, "ABC.jpg"},
		{"mp4 is .mp4", Message{Id: "ABC", MimeType: "video/mp4"}, "ABC.mp4"},
		{"voice note is .ogg", Message{Id: "ABC", MimeType: "audio/ogg; codecs=opus"}, "ABC.ogg"},
		{"audio mp4 is .m4a", Message{Id: "ABC", MimeType: "audio/mp4"}, "ABC.m4a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := downloadFileName(tt.msg); got != tt.want {
				t.Errorf("downloadFileName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSafeJoin(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"plain", "a.txt", false},
		{"dotdot", "..", true},
		{"traversal", "../a.txt", true},
		{"dot", ".", true},
		{"nested traversal", "x/../../a", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := safeJoin(dir, tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("safeJoin(%q) err=%v wantErr=%v", tt.in, err, tt.wantErr)
			}
			if err == nil && filepath.Dir(got) != dir {
				t.Errorf("safeJoin(%q) = %q, not inside %q", tt.in, got, dir)
			}
		})
	}
}

// fakeDownloader writes fixed content and counts calls.
type fakeDownloader struct {
	data  []byte
	err   error
	calls int
}

func (f *fakeDownloader) DownloadToFile(_ context.Context, _ whatsmeow.DownloadableMessage, file whatsmeow.File) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	_, err := file.Write(f.data)
	return err
}

func docMsg(t *testing.T, id, fileName string) Message {
	t.Helper()
	wa := sampleDocMessage()
	wa.DocumentMessage.FileName = proto.String(fileName)
	msg := Message{Id: id, ChatId: "c@s.whatsapp.net"}
	if err := attachMediaInfo(&msg, wa); err != nil {
		t.Fatalf("attachMediaInfo: %v", err)
	}
	return msg
}

func TestDownloadMedia_TraversalStaysInDir(t *testing.T) {
	dir := t.TempDir()
	dl := &fakeDownloader{data: []byte("payload")}
	path, err := downloadMedia(context.Background(), dl, docMsg(t, "M1", "../../../tmp/evil.sh"), dir)
	if err != nil {
		t.Fatalf("downloadMedia: %v", err)
	}
	if path != filepath.Join(dir, "evil.sh") {
		t.Errorf("path = %q, want inside %q", path, dir)
	}
	if b, _ := os.ReadFile(path); string(b) != "payload" {
		t.Errorf("content = %q", b)
	}
	// No temporary files left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

func TestDownloadMedia_CollisionAndReuse(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "report.pdf"), []byte("someone else's"), 0o644); err != nil {
		t.Fatal(err)
	}
	dl := &fakeDownloader{data: []byte("mine")}
	msg := docMsg(t, "M2", "report.pdf")
	path, err := downloadMedia(context.Background(), dl, msg, dir)
	if err != nil {
		t.Fatalf("downloadMedia: %v", err)
	}
	if filepath.Base(path) != "report_M2.pdf" {
		t.Errorf("collision name = %q", filepath.Base(path))
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "report.pdf")); string(b) != "someone else's" {
		t.Error("existing file was overwritten")
	}
	// Second call reuses the ID-specific file without downloading.
	again, err := downloadMedia(context.Background(), dl, msg, dir)
	if err != nil || again != path || dl.calls != 1 {
		t.Errorf("reuse: path=%q err=%v calls=%d", again, err, dl.calls)
	}
}

func TestDownloadMedia_ErrorRemovesTemp(t *testing.T) {
	dir := t.TempDir()
	dl := &fakeDownloader{err: errors.New("boom")}
	if _, err := downloadMedia(context.Background(), dl, docMsg(t, "M3", "a.pdf"), dir); err == nil {
		t.Fatal("expected error")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("expected empty dir after failed download, found %d entries", len(entries))
	}
}

func TestDownloadMedia_NotDownloadable(t *testing.T) {
	_, err := downloadMedia(context.Background(), &fakeDownloader{}, Message{Id: "T", Text: "hi"}, t.TempDir())
	if !errors.Is(err, ErrNotDownloadable) {
		t.Errorf("err = %v, want ErrNotDownloadable", err)
	}
}

func TestFirstURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"see https://example.com/a?b=1.", "https://example.com/a?b=1"},
		{"http://x.org, and more", "http://x.org"},
		{"no link here", ""},
		{"ftp://x.org", ""},
		{"https://", ""},
		{`"https://quoted.example/path"`, "https://quoted.example/path"},
	}
	for _, tt := range tests {
		if got := firstURL(tt.in); got != tt.want {
			t.Errorf("firstURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHandleMessage_StoresMediaAndAppliesRevoke(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.currentReceiver = "c@s.whatsapp.net"
	eh := &eventHandler{sm: sm}
	chat := types.NewJID("c", types.DefaultUserServer)

	eh.handleMessage(&events.Message{
		Info: types.MessageInfo{
			ID:            "MEDIA1",
			MessageSource: types.MessageSource{Chat: chat, Sender: chat},
			Timestamp:     time.Unix(100, 0),
		},
		Message: sampleDocMessage(),
	})
	got, found, err := sm.db.GetMessage("MEDIA1")
	if err != nil || !found || got.MediaKind != MediaKindDocument {
		t.Fatalf("live media not stored: %+v found=%v err=%v", got, found, err)
	}

	eh.handleMessage(&events.Message{
		Info: types.MessageInfo{ID: "REV1", MessageSource: types.MessageSource{Chat: chat, Sender: chat}},
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_REVOKE.Enum(),
			Key:  waCommonKey("MEDIA1"),
		}},
	})
	got, _, _ = sm.db.GetMessage("MEDIA1")
	if got.Text != RevokedText || got.MediaKind != "" || got.MediaProto != nil {
		t.Errorf("revoke not applied: %+v", got)
	}
	if len(mock.Screens) == 0 {
		t.Error("expected screen refresh after revoke in current chat")
	}
	if _, found, _ := sm.db.GetMessage("REV1"); found {
		t.Error("revoke protocol message must not be stored as a message")
	}
}
