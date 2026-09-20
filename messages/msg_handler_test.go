package messages

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestExtractMessageContent_Nil(t *testing.T) {
	text, preview := extractMessageContent(nil)
	if text != "" || preview != "" {
		t.Errorf("expected empty for nil message, got text=%q preview=%q", text, preview)
	}
}

func TestExtractMessageContent_Empty(t *testing.T) {
	msg := &waE2E.Message{}
	text, preview := extractMessageContent(msg)
	if text != "" || preview != "" {
		t.Errorf("expected empty for empty message, got text=%q preview=%q", text, preview)
	}
}

func TestExtractMessageContent_PlainText(t *testing.T) {
	msg := &waE2E.Message{
		Conversation: proto.String("Hello world"),
	}
	text, preview := extractMessageContent(msg)
	if text != "Hello world" {
		t.Errorf("expected text 'Hello world', got %q", text)
	}
	if preview != "Hello world" {
		t.Errorf("expected preview 'Hello world', got %q", preview)
	}
}

func TestExtractMessageContent_ExtendedText(t *testing.T) {
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("Check this link: https://example.com"),
		},
	}
	text, preview := extractMessageContent(msg)
	if text != "Check this link: https://example.com" {
		t.Errorf("expected extended text, got %q", text)
	}
	if preview != "Check this link: https://example.com" {
		t.Errorf("expected preview to equal text for short message, got %q", preview)
	}
}

func TestExtractMessageContent_ImageWithCaption(t *testing.T) {
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			Caption: proto.String("sunset photo"),
		},
	}
	text, preview := extractMessageContent(msg)
	if text != "[IMAGE] sunset photo" {
		t.Errorf("expected '[IMAGE] sunset photo', got %q", text)
	}
	if preview != "[IMAGE] sunset photo" {
		t.Errorf("expected preview '[IMAGE] sunset photo', got %q", preview)
	}
}

func TestExtractMessageContent_ImageNoCaption(t *testing.T) {
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{},
	}
	text, preview := extractMessageContent(msg)
	if text != "[IMAGE]" {
		t.Errorf("expected '[IMAGE]', got %q", text)
	}
	if preview != "[IMAGE]" {
		t.Errorf("expected preview '[IMAGE]', got %q", preview)
	}
}

func TestExtractMessageContent_Video(t *testing.T) {
	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			Caption: proto.String("my video"),
		},
	}
	text, preview := extractMessageContent(msg)
	if text != "[VIDEO] my video" {
		t.Errorf("expected '[VIDEO] my video', got %q", text)
	}
	if preview != "[VIDEO] my video" {
		t.Errorf("expected preview '[VIDEO] my video', got %q", preview)
	}
}

func TestExtractMessageContent_GIF(t *testing.T) {
	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			GifPlayback: proto.Bool(true),
			Caption:     proto.String("funny"),
		},
	}
	text, _ := extractMessageContent(msg)
	if text != "[GIF] funny" {
		t.Errorf("expected '[GIF] funny', got %q", text)
	}
}

func TestExtractMessageContent_VoiceNote(t *testing.T) {
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			PTT:     proto.Bool(true),
			Seconds: proto.Uint32(15),
		},
	}
	text, preview := extractMessageContent(msg)
	if text != "[VOICE NOTE] 15s" {
		t.Errorf("expected '[VOICE NOTE] 15s', got %q", text)
	}
	if preview != "[VOICE NOTE] 15s" {
		t.Errorf("expected preview '[VOICE NOTE] 15s', got %q", preview)
	}
}

func TestExtractMessageContent_Audio(t *testing.T) {
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			PTT:     proto.Bool(false),
			Seconds: proto.Uint32(120),
		},
	}
	text, _ := extractMessageContent(msg)
	if text != "[AUDIO] 120s" {
		t.Errorf("expected '[AUDIO] 120s', got %q", text)
	}
}

func TestExtractMessageContent_Document(t *testing.T) {
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			FileName: proto.String("report.pdf"),
		},
	}
	text, preview := extractMessageContent(msg)
	if text != "[DOCUMENT] report.pdf" {
		t.Errorf("expected '[DOCUMENT] report.pdf', got %q", text)
	}
	if preview != "[DOCUMENT] report.pdf" {
		t.Errorf("expected preview '[DOCUMENT] report.pdf', got %q", preview)
	}
}

func TestExtractMessageContent_DocumentNoFilename(t *testing.T) {
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			Title: proto.String("My Report"),
		},
	}
	text, _ := extractMessageContent(msg)
	if text != "[DOCUMENT] My Report" {
		t.Errorf("expected '[DOCUMENT] My Report', got %q", text)
	}
}

func TestExtractMessageContent_Sticker(t *testing.T) {
	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{},
	}
	text, preview := extractMessageContent(msg)
	if text != "[STICKER]" {
		t.Errorf("expected '[STICKER]', got %q", text)
	}
	if preview != "[STICKER]" {
		t.Errorf("expected preview '[STICKER]', got %q", preview)
	}
}

func TestExtractMessageContent_Contact(t *testing.T) {
	msg := &waE2E.Message{
		ContactMessage: &waE2E.ContactMessage{
			DisplayName: proto.String("John Doe"),
		},
	}
	text, preview := extractMessageContent(msg)
	if text != "[CONTACT] John Doe" {
		t.Errorf("expected '[CONTACT] John Doe', got %q", text)
	}
	if preview != "[CONTACT] John Doe" {
		t.Errorf("expected preview '[CONTACT] John Doe', got %q", preview)
	}
}

func TestExtractMessageContent_Location(t *testing.T) {
	msg := &waE2E.Message{
		LocationMessage: &waE2E.LocationMessage{
			DegreesLatitude:  proto.Float64(37.7749),
			DegreesLongitude: proto.Float64(-122.4194),
			Name:             proto.String("San Francisco"),
		},
	}
	text, preview := extractMessageContent(msg)
	if !strings.Contains(text, "[LOCATION] San Francisco") {
		t.Errorf("expected text to contain '[LOCATION] San Francisco', got %q", text)
	}
	if !strings.Contains(text, "37.7749") {
		t.Errorf("expected text to contain coordinates, got %q", text)
	}
	if preview != "[LOCATION] San Francisco" {
		t.Errorf("expected preview '[LOCATION] San Francisco', got %q", preview)
	}
}

func TestExtractMessageContent_LocationNoName(t *testing.T) {
	msg := &waE2E.Message{
		LocationMessage: &waE2E.LocationMessage{
			DegreesLatitude:  proto.Float64(1.2345),
			DegreesLongitude: proto.Float64(6.7890),
		},
	}
	text, preview := extractMessageContent(msg)
	if !strings.Contains(text, "[LOCATION]") {
		t.Errorf("expected text to contain '[LOCATION]', got %q", text)
	}
	if !strings.Contains(text, "1.2345") {
		t.Errorf("expected text to contain latitude, got %q", text)
	}
	if text != preview {
		t.Errorf("expected text == preview for unnamed location, got text=%q preview=%q", text, preview)
	}
}

func TestExtractMessageContent_Reaction(t *testing.T) {
	msg := &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Text: proto.String("👍"),
		},
	}
	text, preview := extractMessageContent(msg)
	if text != "[REACTION] 👍" {
		t.Errorf("expected '[REACTION] 👍', got %q", text)
	}
	if preview != "[REACTION] 👍" {
		t.Errorf("expected preview '[REACTION] 👍', got %q", preview)
	}
}

func TestTruncatePreview_Short(t *testing.T) {
	result := truncatePreview("hello")
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestTruncatePreview_Long(t *testing.T) {
	long := strings.Repeat("a", 100)
	result := truncatePreview(long)
	if len([]rune(result)) != 81 { // 80 chars + ellipsis
		t.Errorf("expected 81 runes (80 + ellipsis), got %d", len([]rune(result)))
	}
	if !strings.HasSuffix(result, "…") {
		t.Errorf("expected ellipsis suffix, got %q", result)
	}
}

func TestExtractMessageContent_ExtendedTextPriority(t *testing.T) {
	// When both Conversation and ExtendedTextMessage are set,
	// ExtendedTextMessage should take priority.
	msg := &waE2E.Message{
		Conversation: proto.String("plain text fallback"),
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("extended takes priority"),
		},
	}
	text, _ := extractMessageContent(msg)
	if text != "extended takes priority" {
		t.Errorf("expected ExtendedTextMessage to take priority, got %q", text)
	}
}

// ----- Phase 2.2: extractMediaInfo tests -----

func TestExtractMediaInfo_Nil(t *testing.T) {
	info, ok := extractMediaInfo(nil)
	if ok {
		t.Error("expected ok=false for nil message")
	}
	if info.mediaType != "" {
		t.Errorf("expected empty mediaType for nil message, got %q", info.mediaType)
	}
}

func TestExtractMediaInfo_PlainTextHasNoMedia(t *testing.T) {
	msg := &waE2E.Message{Conversation: proto.String("just text")}
	info, ok := extractMediaInfo(msg)
	if ok {
		t.Error("expected ok=false for a plain text message")
	}
	if info.mediaType != "" {
		t.Errorf("expected empty mediaType, got %q", info.mediaType)
	}
}

func TestExtractMediaInfo_Image(t *testing.T) {
	mediaKey := []byte{0x01, 0x02, 0x03}
	fileSHA := []byte{0xAA}
	fileEncSHA := []byte{0xBB}
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			Mimetype:      proto.String("image/jpeg"),
			MediaKey:      mediaKey,
			DirectPath:    proto.String("/v/t62.7118-24/img"),
			FileSHA256:    fileSHA,
			FileEncSHA256: fileEncSHA,
			FileLength:    proto.Uint64(4096),
		},
	}
	info, ok := extractMediaInfo(msg)
	if !ok {
		t.Fatal("expected ok=true for an image message")
	}
	if info.mediaType != "image" {
		t.Errorf("expected mediaType 'image', got %q", info.mediaType)
	}
	if info.mimetype != "image/jpeg" {
		t.Errorf("expected mimetype 'image/jpeg', got %q", info.mimetype)
	}
	if string(info.mediaKey) != string(mediaKey) {
		t.Errorf("mediaKey mismatch: expected %v, got %v", mediaKey, info.mediaKey)
	}
	if info.directPath != "/v/t62.7118-24/img" {
		t.Errorf("directPath mismatch: got %q", info.directPath)
	}
	if string(info.fileSHA256) != string(fileSHA) || string(info.fileEncSHA256) != string(fileEncSHA) {
		t.Errorf("SHA256 mismatch: fileSHA256=%v fileEncSHA256=%v", info.fileSHA256, info.fileEncSHA256)
	}
	if info.fileLength != 4096 {
		t.Errorf("expected fileLength 4096, got %d", info.fileLength)
	}
}

func TestExtractMediaInfo_Video(t *testing.T) {
	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			Mimetype:   proto.String("video/mp4"),
			MediaKey:   []byte{0x10},
			DirectPath: proto.String("/v/vid"),
			FileLength: proto.Uint64(1000),
		},
	}
	info, ok := extractMediaInfo(msg)
	if !ok || info.mediaType != "video" {
		t.Fatalf("expected mediaType 'video', got %q (ok=%v)", info.mediaType, ok)
	}
	if info.mimetype != "video/mp4" {
		t.Errorf("expected mimetype 'video/mp4', got %q", info.mimetype)
	}
}

func TestExtractMediaInfo_Audio(t *testing.T) {
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			Mimetype:   proto.String("audio/ogg"),
			MediaKey:   []byte{0x20},
			DirectPath: proto.String("/v/aud"),
			FileLength: proto.Uint64(500),
		},
	}
	info, ok := extractMediaInfo(msg)
	if !ok || info.mediaType != "audio" {
		t.Fatalf("expected mediaType 'audio', got %q (ok=%v)", info.mediaType, ok)
	}
}

func TestExtractMediaInfo_Document(t *testing.T) {
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			Mimetype:   proto.String("application/pdf"),
			FileName:   proto.String("report.pdf"),
			MediaKey:   []byte{0x30},
			DirectPath: proto.String("/v/doc"),
			FileLength: proto.Uint64(2048),
		},
	}
	info, ok := extractMediaInfo(msg)
	if !ok || info.mediaType != "document" {
		t.Fatalf("expected mediaType 'document', got %q (ok=%v)", info.mediaType, ok)
	}
	if info.fileName != "report.pdf" {
		t.Errorf("expected fileName 'report.pdf', got %q", info.fileName)
	}
}

func TestExtractMediaInfo_Document_FileNameFallsBackToTitle(t *testing.T) {
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			Title: proto.String("My Report"),
		},
	}
	info, ok := extractMediaInfo(msg)
	if !ok {
		t.Fatal("expected ok=true for a document message")
	}
	if info.fileName != "My Report" {
		t.Errorf("expected fileName to fall back to title 'My Report', got %q", info.fileName)
	}
}

// ----- Phase 3.3: extractForwarded tests -----

func TestExtractForwarded_Nil(t *testing.T) {
	if extractForwarded(nil) {
		t.Error("expected false for nil message")
	}
}

func TestExtractForwarded_PlainTextNeverForwarded(t *testing.T) {
	// Conversation (plain text) has no ContextInfo in the protocol at all,
	// so a bare plain-text message can never be reported as forwarded.
	msg := &waE2E.Message{Conversation: proto.String("hello")}
	if extractForwarded(msg) {
		t.Error("expected false for a plain Conversation message")
	}
}

func TestExtractForwarded_ExtendedText(t *testing.T) {
	tests := []struct {
		name string
		ctx  *waE2E.ContextInfo
		want bool
	}{
		{"forwarded", &waE2E.ContextInfo{IsForwarded: proto.Bool(true)}, true},
		{"not forwarded", &waE2E.ContextInfo{IsForwarded: proto.Bool(false)}, false},
		{"no context info", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text:        proto.String("hi"),
					ContextInfo: tt.ctx,
				},
			}
			if got := extractForwarded(msg); got != tt.want {
				t.Errorf("extractForwarded() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractForwarded_Image(t *testing.T) {
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			ContextInfo: &waE2E.ContextInfo{IsForwarded: proto.Bool(true)},
		},
	}
	if !extractForwarded(msg) {
		t.Error("expected true for a forwarded image message")
	}
}

func TestExtractForwarded_Video(t *testing.T) {
	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			ContextInfo: &waE2E.ContextInfo{IsForwarded: proto.Bool(true)},
		},
	}
	if !extractForwarded(msg) {
		t.Error("expected true for a forwarded video message")
	}
}

func TestExtractForwarded_Audio(t *testing.T) {
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			ContextInfo: &waE2E.ContextInfo{IsForwarded: proto.Bool(true)},
		},
	}
	if !extractForwarded(msg) {
		t.Error("expected true for a forwarded audio message")
	}
}

func TestExtractForwarded_Document(t *testing.T) {
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			ContextInfo: &waE2E.ContextInfo{IsForwarded: proto.Bool(true)},
		},
	}
	if !extractForwarded(msg) {
		t.Error("expected true for a forwarded document message")
	}
}

func TestExtractForwarded_Sticker(t *testing.T) {
	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			ContextInfo: &waE2E.ContextInfo{IsForwarded: proto.Bool(true)},
		},
	}
	if !extractForwarded(msg) {
		t.Error("expected true for a forwarded sticker message")
	}
}

func TestExtractMediaInfo_Sticker(t *testing.T) {
	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			Mimetype:   proto.String("image/webp"),
			MediaKey:   []byte{0x40},
			DirectPath: proto.String("/v/stk"),
		},
	}
	info, ok := extractMediaInfo(msg)
	if !ok || info.mediaType != "sticker" {
		t.Fatalf("expected mediaType 'sticker', got %q (ok=%v)", info.mediaType, ok)
	}
}

// ----- Phase 5.1: reply threading extraction -----

func TestExtractQuotedID_Nil(t *testing.T) {
	if got := extractQuotedID(nil); got != "" {
		t.Errorf("expected empty string for nil message, got %q", got)
	}
}

func TestExtractQuotedID_PlainTextNeverAReply(t *testing.T) {
	msg := &waE2E.Message{Conversation: proto.String("hello")}
	if got := extractQuotedID(msg); got != "" {
		t.Errorf("expected empty string for a plain Conversation message, got %q", got)
	}
}

func TestExtractQuotedID_ExtendedText(t *testing.T) {
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String("reply text"),
			ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("original-id")},
		},
	}
	if got := extractQuotedID(msg); got != "original-id" {
		t.Errorf("extractQuotedID() = %q, want %q", got, "original-id")
	}
}

func TestExtractQuotedID_NoContextInfo(t *testing.T) {
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("not a reply")},
	}
	if got := extractQuotedID(msg); got != "" {
		t.Errorf("expected empty string with no ContextInfo, got %q", got)
	}
}

func TestExtractQuotedID_Image(t *testing.T) {
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("img-reply-target")},
		},
	}
	if got := extractQuotedID(msg); got != "img-reply-target" {
		t.Errorf("extractQuotedID() = %q, want %q", got, "img-reply-target")
	}
}

func TestExtractEmbeddedQuotePreview_NoQuote(t *testing.T) {
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String("hi"),
			ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("id1")},
		},
	}
	if got := extractEmbeddedQuotePreview(msg); got != "" {
		t.Errorf("expected empty preview with no embedded QuotedMessage, got %q", got)
	}
}

func TestExtractEmbeddedQuotePreview_EmbeddedText(t *testing.T) {
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("sure, sounds good"),
			ContextInfo: &waE2E.ContextInfo{
				StanzaID:      proto.String("id1"),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("want to grab lunch?")},
			},
		},
	}
	if got := extractEmbeddedQuotePreview(msg); got != "want to grab lunch?" {
		t.Errorf("extractEmbeddedQuotePreview() = %q, want %q", got, "want to grab lunch?")
	}
}

func TestResolveQuote_NotAReply(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	msg := &waE2E.Message{Conversation: proto.String("hello")}
	id, preview := resolveQuote(sm.db, msg)
	if id != "" || preview != "" {
		t.Errorf("resolveQuote() = (%q, %q), want (\"\", \"\")", id, preview)
	}
}

func TestResolveQuote_PrefersLocalCopyOverEmbedded(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	if err := sm.db.AddMessage(Message{Id: "orig1", ChatId: "c@s.whatsapp.net", Text: "the real current text"}); err != nil {
		t.Fatalf("seed AddMessage failed: %v", err)
	}
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("replying"),
			ContextInfo: &waE2E.ContextInfo{
				StanzaID:      proto.String("orig1"),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("stale embedded copy")},
			},
		},
	}
	id, preview := resolveQuote(sm.db, msg)
	if id != "orig1" {
		t.Errorf("expected quoted ID %q, got %q", "orig1", id)
	}
	if preview != "the real current text" {
		t.Errorf("expected preview from local DB, got %q", preview)
	}
}

func TestResolveQuote_FallsBackToEmbeddedWhenNotStoredLocally(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("replying"),
			ContextInfo: &waE2E.ContextInfo{
				StanzaID:      proto.String("unknown-id"),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("embedded original")},
			},
		},
	}
	id, preview := resolveQuote(sm.db, msg)
	if id != "unknown-id" || preview != "embedded original" {
		t.Errorf("resolveQuote() = (%q, %q), want (%q, %q)", id, preview, "unknown-id", "embedded original")
	}
}

func TestResolveQuote_PlaceholderWhenNeitherAvailable(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String("replying"),
			ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("unknown-id")},
		},
	}
	id, preview := resolveQuote(sm.db, msg)
	if id != "unknown-id" || preview != "[message]" {
		t.Errorf("resolveQuote() = (%q, %q), want (%q, %q)", id, preview, "unknown-id", "[message]")
	}
}

func TestExtractForwarded_ContactAndLocation(t *testing.T) {
	fwd := &waE2E.ContextInfo{IsForwarded: proto.Bool(true)}

	cases := []struct {
		name string
		msg  *waE2E.Message
		want bool
	}{
		{
			name: "forwarded contact",
			msg: &waE2E.Message{ContactMessage: &waE2E.ContactMessage{
				DisplayName: proto.String("Alice"),
				ContextInfo: fwd,
			}},
			want: true,
		},
		{
			name: "non-forwarded contact",
			msg: &waE2E.Message{ContactMessage: &waE2E.ContactMessage{
				DisplayName: proto.String("Alice"),
			}},
			want: false,
		},
		{
			name: "forwarded location",
			msg: &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
				DegreesLatitude: proto.Float64(1.5),
				ContextInfo:     fwd,
			}},
			want: true,
		},
		{
			name: "non-forwarded location",
			msg: &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
				DegreesLatitude: proto.Float64(1.5),
			}},
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractForwarded(c.msg); got != c.want {
				t.Errorf("extractForwarded = %v, want %v", got, c.want)
			}
		})
	}
}
