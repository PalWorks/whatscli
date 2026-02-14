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
