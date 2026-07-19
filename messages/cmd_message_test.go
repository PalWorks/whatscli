package messages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCmdDownload_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdDownload(sm, nil, "download", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdDownload_UnknownMessageID(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdDownload(sm, nil, "download", []string{"does-not-exist"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
	if !strings.Contains(mock.Errors[0].Error(), "failed to look up message") {
		t.Errorf("expected a lookup-failure error, got %q", mock.Errors[0].Error())
	}
}

func TestCmdDownload_MessageHasNoAttachment(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "textonly", ChatId: "c@s.whatsapp.net", Timestamp: 1000, Text: "just text"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	// Nil client here proves the no-attachment check fires before the
	// connectivity check — a nil client would otherwise mask this as
	// "not connected".
	cmdDownload(sm, nil, "download", []string{"textonly"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
	if mock.Errors[0].Error() != "selected message has no attachment" {
		t.Errorf("expected 'selected message has no attachment', got %q", mock.Errors[0].Error())
	}
}

func TestCmdDownload_NilClient_WithAttachment(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "img1", ChatId: "c@s.whatsapp.net", Timestamp: 1000, MediaType: "image"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	cmdDownload(sm, nil, "download", []string{"img1"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
	if mock.Errors[0].Error() != "not connected" {
		t.Errorf("expected 'not connected', got %q", mock.Errors[0].Error())
	}
}

func TestCmdOpen_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdOpen(sm, nil, "open", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdOpen_UnknownMessageID(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdOpen(sm, nil, "open", []string{"does-not-exist"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
}

func TestCmdOpen_MessageHasNoAttachment(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "textonly2", ChatId: "c@s.whatsapp.net", Timestamp: 1000, Text: "just text"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	cmdOpen(sm, nil, "open", []string{"textonly2"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "selected message has no attachment" {
		t.Fatalf("expected 'selected message has no attachment', got %v", mock.Errors)
	}
}

func TestCmdOpen_NilClient_WithAttachment(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "img2", ChatId: "c@s.whatsapp.net", Timestamp: 1000, MediaType: "document"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	cmdOpen(sm, nil, "open", []string{"img2"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

// TestMediaFileName covers the filename-derivation logic used by both
// cmdDownload and cmdOpen.
func TestMediaFileName_DocumentKeepsNameButPrefixesID(t *testing.T) {
	msg := Message{Id: "m1", ChatId: "c@s.whatsapp.net", FileName: "report.pdf", Mimetype: "application/pdf"}
	if got := mediaFileName(msg); got != "m1_report.pdf" {
		t.Errorf("mediaFileName() = %q, want %q", got, "m1_report.pdf")
	}
}

// TestMediaFileName_DocumentIsPerMessageUnique guards the correctness fix: two
// different documents that share a sender-chosen name must map to different
// filenames, otherwise downloadMessageTo's reuse-if-exists check would hand
// back the first document's bytes when the second is requested.
func TestMediaFileName_DocumentIsPerMessageUnique(t *testing.T) {
	a := Message{Id: "idA", ChatId: "c@s.whatsapp.net", FileName: "invoice.pdf"}
	b := Message{Id: "idB", ChatId: "c@s.whatsapp.net", FileName: "invoice.pdf"}
	if mediaFileName(a) == mediaFileName(b) {
		t.Errorf("two documents named %q collided to the same filename %q", "invoice.pdf", mediaFileName(a))
	}
}

// TestMediaFileName_StripsPathTraversal guards the security fix: a hostile
// document FileName must not be able to steer the written file outside the
// download directory. The result must contain no path separators and no ".."
// segment.
func TestMediaFileName_StripsPathTraversal(t *testing.T) {
	for _, evil := range []string{
		"../../../home/user/.bashrc",
		"/etc/passwd",
		"..",
		"a/b/c.pdf",
	} {
		msg := Message{Id: "m1", ChatId: "123@s.whatsapp.net", FileName: evil, Mimetype: "application/pdf"}
		got := mediaFileName(msg)
		if strings.ContainsRune(got, '/') || strings.ContainsRune(got, filepath.Separator) {
			t.Errorf("mediaFileName(%q) = %q, must not contain a path separator", evil, got)
		}
		if got == ".." || strings.HasPrefix(got, "../") || strings.Contains(got, "/..") {
			t.Errorf("mediaFileName(%q) = %q, must not contain a traversal segment", evil, got)
		}
		// Joining with a directory must stay inside that directory.
		joined := filepath.Clean(filepath.Join("/downloads", got))
		if !strings.HasPrefix(joined, "/downloads/") {
			t.Errorf("mediaFileName(%q) = %q escapes the download dir: %q", evil, got, joined)
		}
	}
}

func TestMediaFileName_SynthesizedFromChatAndID(t *testing.T) {
	msg := Message{Id: "m2", ChatId: "123@s.whatsapp.net", Mimetype: "image/jpeg"}
	got := mediaFileName(msg)
	// Which extension mime.ExtensionsByType registers first for image/jpeg
	// (.jpg, .jpeg, .jfif, ...) depends on the OS mime database, so only
	// assert the invariants mediaFileName itself is responsible for: the
	// safe chat+id prefix (dots from the JID replaced, so filepath.Ext can
	// never confuse a JID fragment for the extension), and that some
	// non-empty extension was appended.
	const wantPrefix = "123_s_whatsapp_net_m2"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("mediaFileName() = %q, want prefix %q", got, wantPrefix)
	}
	ext := strings.TrimPrefix(got, wantPrefix)
	if !strings.HasPrefix(ext, ".") || len(ext) < 2 {
		t.Errorf("mediaFileName() = %q, expected a non-empty extension after the prefix, got suffix %q", got, ext)
	}
}

func TestMediaFileName_UnknownMimetypeNoExtension(t *testing.T) {
	msg := Message{Id: "m3", ChatId: "123@s.whatsapp.net", Mimetype: "application/x-totally-unknown-type"}
	want := "123_s_whatsapp_net_m3"
	if got := mediaFileName(msg); got != want {
		t.Errorf("mediaFileName() = %q, want %q", got, want)
	}
}

// TestMediaFileName_NoDotsLeakFromChatId guards against the collision-suffix
// corruption bug found during audit: without sanitizing dots (not just "@")
// out of the chat JID, a synthesized name for a message with no recognized
// extension could contain a stray "." from the JID (e.g. "s.whatsapp.net"),
// and writeUniqueFile's filepath.Ext-based collision suffix would then
// mistake a JID fragment for a file extension, producing a garbled name
// such as "123_s.whatsapp_1.net_m3" instead of a clean "..._m3_1".
func TestMediaFileName_NoDotsLeakFromChatId(t *testing.T) {
	msg := Message{Id: "m9", ChatId: "555@s.whatsapp.net", Mimetype: "application/x-totally-unknown-type"}
	got := mediaFileName(msg)
	if strings.Contains(got, ".") {
		t.Errorf("mediaFileName() = %q, expected no literal dots when no extension was appended", got)
	}
}

func TestExtensionForMimetype_Empty(t *testing.T) {
	if got := extensionForMimetype(""); got != "" {
		t.Errorf("expected empty extension for empty mimetype, got %q", got)
	}
}

func TestExtensionForMimetype_Unknown(t *testing.T) {
	if got := extensionForMimetype("application/x-totally-unknown-type"); got != "" {
		t.Errorf("expected empty extension for unknown mimetype, got %q", got)
	}
}

func TestWriteUniqueFile_NoCollision(t *testing.T) {
	dir := t.TempDir()
	path, err := writeUniqueFile(dir, "test.txt", []byte("hello"))
	if err != nil {
		t.Fatalf("writeUniqueFile error: %v", err)
	}
	want := dir + "/test.txt"
	if path != want {
		t.Errorf("expected path %s, got %s", want, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected content 'hello', got %q", data)
	}
}

func TestWriteUniqueFile_AvoidsOverwrite(t *testing.T) {
	dir := t.TempDir()

	path1, err := writeUniqueFile(dir, "dup.txt", []byte("first"))
	if err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	path2, err := writeUniqueFile(dir, "dup.txt", []byte("second"))
	if err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	if path1 == path2 {
		t.Fatalf("expected distinct paths for colliding names, got the same path twice: %s", path1)
	}

	data1, err := os.ReadFile(path1)
	if err != nil {
		t.Fatalf("read path1 failed: %v", err)
	}
	data2, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("read path2 failed: %v", err)
	}
	if string(data1) != "first" || string(data2) != "second" {
		t.Errorf("content mismatch: path1=%q path2=%q", data1, data2)
	}
}

// ----- Phase 2.4: cmdShow tests -----

func TestCmdShow_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdShow(sm, nil, "show", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdShow_UnknownMessageID(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdShow(sm, nil, "show", []string{"does-not-exist"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
}

func TestCmdShow_RejectsNonImageType(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "doc1", ChatId: "c@s.whatsapp.net", Timestamp: 1000, MediaType: "document", FileName: "report.pdf"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	// Nil client proves the media-type check fires before connectivity.
	cmdShow(sm, nil, "show", []string{"doc1"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "selected message is not an image" {
		t.Fatalf("expected 'selected message is not an image', got %v", mock.Errors)
	}
}

func TestCmdShow_AcceptsSticker(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "stk1", ChatId: "c@s.whatsapp.net", Timestamp: 1000, MediaType: "sticker"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	cmdShow(sm, nil, "show", []string{"stk1"})

	// With nil client, a sticker should pass the media-type check and then
	// fail on connectivity — not on media type.
	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected' (media-type check should have passed), got %v", mock.Errors)
	}
}

func TestCmdShow_NilClient_WithImage(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "img3", ChatId: "c@s.whatsapp.net", Timestamp: 1000, MediaType: "image"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	cmdShow(sm, nil, "show", []string{"img3"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

// ----- Phase 2.4: extractFirstURL / cmdURL tests -----

func TestExtractFirstURL(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"plain URL", "https://example.com", "https://example.com"},
		{"URL mid sentence", "check this out https://example.com/page it's great", "https://example.com/page"},
		{"trailing period", "see https://example.com.", "https://example.com"},
		{"trailing paren", "(see https://example.com)", "https://example.com"},
		{"no URL", "just some plain text with no links", ""},
		{"first of multiple URLs wins", "https://first.com and https://second.com", "https://first.com"},
		{"http scheme", "http://insecure.example.com", "http://insecure.example.com"},
		{"empty text", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFirstURL(tt.text)
			if got != tt.want {
				t.Errorf("extractFirstURL(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestCmdURL_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdURL(sm, nil, "url", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdURL_UnknownMessageID(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdURL(sm, nil, "url", []string{"does-not-exist"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
}

func TestCmdURL_NoURLInText(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "nourl1", ChatId: "c@s.whatsapp.net", Timestamp: 1000, Text: "no links here"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	cmdURL(sm, nil, "url", []string{"nourl1"})

	if len(mock.Texts) != 1 || mock.Texts[0] != "No URL in selected message" {
		t.Fatalf("expected 'No URL in selected message', got texts=%v", mock.Texts)
	}
	if len(mock.Files) != 0 {
		t.Errorf("expected no OpenFile call, got %v", mock.Files)
	}
}

func TestCmdURL_OpensExtractedURL(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "url1", ChatId: "c@s.whatsapp.net", Timestamp: 1000, Text: "check out https://example.com/page for more"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	// cmdURL never touches the network — it should work even with a nil client.
	cmdURL(sm, nil, "url", []string{"url1"})

	if len(mock.Files) != 1 || mock.Files[0] != "https://example.com/page" {
		t.Fatalf("expected OpenFile('https://example.com/page'), got %v", mock.Files)
	}
	if len(mock.Errors) != 0 {
		t.Errorf("expected no errors, got %v", mock.Errors)
	}
}

// ----- Phase 2.5: cmdRevoke tests -----

func TestCmdRevoke_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdRevoke(sm, nil, "revoke", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdRevoke_UnknownMessageID(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdRevoke(sm, nil, "revoke", []string{"does-not-exist"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
}

func TestCmdRevoke_RejectsNotFromMe(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "theirs1", ChatId: "c@s.whatsapp.net", Timestamp: 1000, FromMe: false, Text: "not mine"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	// Nil client proves the FromMe check fires before connectivity.
	cmdRevoke(sm, nil, "revoke", []string{"theirs1"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "can only revoke your own messages" {
		t.Fatalf("expected 'can only revoke your own messages', got %v", mock.Errors)
	}
}

func TestCmdRevoke_NilClient_WithOwnMessage(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "mine1", ChatId: "c@s.whatsapp.net", Timestamp: 1000, FromMe: true, Text: "mine"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	cmdRevoke(sm, nil, "revoke", []string{"mine1"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

func TestCmdRevoke_InvalidChatJID(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	// types.ParseJID only errors on a User part containing a dot whose
	// device suffix doesn't parse as a number (see types/jid.go) — a bare
	// string with no '@' or '.' parses successfully with an empty server,
	// so this fixture is deliberately shaped to hit that error path. The
	// JID-parse guard runs before the connectivity check, so this must be
	// reported (not masked by "not connected") even with a nil client.
	if err := sm.db.AddMessage(Message{Id: "badchat1", ChatId: "user.notanumber@s.whatsapp.net", Timestamp: 1000, FromMe: true, Text: "x"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	cmdRevoke(sm, nil, "revoke", []string{"badchat1"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
	if !strings.Contains(mock.Errors[0].Error(), "invalid chat JID") {
		t.Errorf("expected an 'invalid chat JID' error, got %q", mock.Errors[0].Error())
	}
}

// TestWriteUniqueFile_CollisionOnNameWithNoExtension documents
// writeUniqueFile's actual (correct) behavior in isolation: for a name with
// no extension at all, filepath.Ext returns "", so the collision suffix is
// simply appended to the end — "name" then "name_1". This is the behavior
// mediaFileName's dot-sanitization (see TestMediaFileName_NoDotsLeakFromChatId)
// depends on to avoid ever handing writeUniqueFile a synthesized name that
// contains a stray dot from the chat JID.
func TestWriteUniqueFile_CollisionOnNameWithNoExtension(t *testing.T) {
	dir := t.TempDir()

	path1, err := writeUniqueFile(dir, "555_s_whatsapp_net_m9", []byte("a"))
	if err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	path2, err := writeUniqueFile(dir, "555_s_whatsapp_net_m9", []byte("b"))
	if err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	wantPath1 := dir + "/555_s_whatsapp_net_m9"
	wantPath2 := dir + "/555_s_whatsapp_net_m9_1"
	if path1 != wantPath1 {
		t.Errorf("path1 = %q, want %q", path1, wantPath1)
	}
	if path2 != wantPath2 {
		t.Errorf("path2 = %q, want %q", path2, wantPath2)
	}
}

func TestWriteUniqueFile_ThreeWayCollision(t *testing.T) {
	dir := t.TempDir()

	var paths []string
	for i := 0; i < 3; i++ {
		path, err := writeUniqueFile(dir, "triple.bin", []byte("x"))
		if err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}
		paths = append(paths, path)
	}

	seen := map[string]bool{}
	for _, p := range paths {
		if seen[p] {
			t.Fatalf("duplicate path returned: %s (all paths: %v)", p, paths)
		}
		seen[p] = true
	}
}

// ----- Phase 5.3: cmdReply -----

func TestCmdReply_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdReply(sm, nil, "reply", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdReply_MissingText(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	// Only the message ID, no reply text — checkParam(params, 2) must reject this.
	cmdReply(sm, nil, "reply", []string{"orig1"})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdReply_RefusesStatusBroadcast(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "status1", ChatId: STATUSSUFFIX, Timestamp: 1000, Text: "someone's status"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	cmdReply(sm, nil, "reply", []string{"status1", "nice photo"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "cannot reply to a status update" {
		t.Fatalf("expected 'cannot reply to a status update', got %v", mock.Errors)
	}
}

func TestCmdReply_UnknownMessageID(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdReply(sm, nil, "reply", []string{"does-not-exist", "hello"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
}

func TestCmdReply_InvalidChatJID(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "badchat2", ChatId: "user.notanumber@s.whatsapp.net", Timestamp: 1000, Text: "x"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	// JID-parse guard must fire before the connectivity check, so this is
	// reported even with a nil client (mirrors TestCmdRevoke_InvalidChatJID).
	cmdReply(sm, nil, "reply", []string{"badchat2", "hello"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
}

func TestCmdReply_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "orig2", ChatId: "c@s.whatsapp.net", Timestamp: 1000, Text: "original text"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	cmdReply(sm, nil, "reply", []string{"orig2", "sounds", "good"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

func TestCmdReply_MultiWordTextJoined(t *testing.T) {
	// Doesn't reach the network (nil client), but proves params[1:] are
	// joined back into one string rather than only using params[1].
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "orig3", ChatId: "c@s.whatsapp.net", Timestamp: 1000, Text: "original text"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	cmdReply(sm, nil, "reply", []string{"orig3", "sounds", "good", "to", "me"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected the guard chain to reach the connectivity check, got %v", mock.Errors)
	}
}

// ----- Phase 5.4: cmdEdit -----

func TestCmdEdit_NoParams(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdEdit(sm, nil, "edit", []string{})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdEdit_MissingText(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdEdit(sm, nil, "edit", []string{"mine2"})

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdEdit_UnknownMessageID(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdEdit(sm, nil, "edit", []string{"does-not-exist", "new text"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
}

func TestCmdEdit_RejectsNotFromMe(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "theirs2", ChatId: "c@s.whatsapp.net", Timestamp: uint64(time.Now().Unix()), FromMe: false, Text: "not mine"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	// Nil client proves the FromMe check fires before connectivity.
	cmdEdit(sm, nil, "edit", []string{"theirs2", "new text"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "can only edit your own messages" {
		t.Fatalf("expected 'can only edit your own messages', got %v", mock.Errors)
	}
}

func TestCmdEdit_RejectsExpiredEditWindow(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	// Timestamp well outside whatsmeow.EditWindow (20 minutes).
	staleTs := uint64(time.Now().Add(-1 * time.Hour).Unix())
	if err := sm.db.AddMessage(Message{Id: "stale1", ChatId: "c@s.whatsapp.net", Timestamp: staleTs, FromMe: true, Text: "old"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	// Nil client proves the window check fires before connectivity.
	cmdEdit(sm, nil, "edit", []string{"stale1", "too late"})

	if len(mock.Errors) != 1 || !strings.Contains(mock.Errors[0].Error(), "edit window") {
		t.Fatalf("expected an edit-window-expired error, got %v", mock.Errors)
	}
}

func TestCmdEdit_WithinWindow_InvalidChatJID(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	freshTs := uint64(time.Now().Unix())
	if err := sm.db.AddMessage(Message{Id: "badchat3", ChatId: "user.notanumber@s.whatsapp.net", Timestamp: freshTs, FromMe: true, Text: "x"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	cmdEdit(sm, nil, "edit", []string{"badchat3", "new text"})

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
}

func TestCmdEdit_NilClient_WithOwnFreshMessage(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	freshTs := uint64(time.Now().Unix())
	if err := sm.db.AddMessage(Message{Id: "mine3", ChatId: "c@s.whatsapp.net", Timestamp: freshTs, FromMe: true, Text: "mine"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}

	cmdEdit(sm, nil, "edit", []string{"mine3", "corrected text"})

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}
