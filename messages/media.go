package messages

// media.go — media metadata extraction, safe download file naming and URL
// detection used by the download/open/show/url commands (Phase 2a port of
// upstream normen/whatscli behaviour, reimplemented for the fork's layout).

import (
	"errors"
	"fmt"
	"mime"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// Media kinds stored in messages.media_kind.
const (
	MediaKindImage    = "image"
	MediaKindVideo    = "video"
	MediaKindAudio    = "audio"
	MediaKindDocument = "document"
	MediaKindSticker  = "sticker"
)

// ErrNotDownloadable is returned for messages without stored media metadata.
var ErrNotDownloadable = errors.New("this is not a downloadable message")

// extractMediaInfo returns the media kind, mimetype, original file name and a
// serialized waE2E.Message holding only the media sub-message. The inline
// JPEG thumbnail is dropped to keep rows small; it is not needed to download.
// Returns empty values (and nil error) for non-media messages.
func extractMediaInfo(msg *waE2E.Message) (kind, mimeType, fileName string, raw []byte, err error) {
	if msg == nil {
		return "", "", "", nil, nil
	}
	var minimal *waE2E.Message
	switch {
	case msg.GetImageMessage() != nil:
		m := proto.Clone(msg.GetImageMessage()).(*waE2E.ImageMessage)
		m.JPEGThumbnail = nil
		kind, mimeType = MediaKindImage, m.GetMimetype()
		minimal = &waE2E.Message{ImageMessage: m}
	case msg.GetVideoMessage() != nil:
		m := proto.Clone(msg.GetVideoMessage()).(*waE2E.VideoMessage)
		m.JPEGThumbnail = nil
		kind, mimeType = MediaKindVideo, m.GetMimetype()
		minimal = &waE2E.Message{VideoMessage: m}
	case msg.GetAudioMessage() != nil:
		m := proto.Clone(msg.GetAudioMessage()).(*waE2E.AudioMessage)
		kind, mimeType = MediaKindAudio, m.GetMimetype()
		minimal = &waE2E.Message{AudioMessage: m}
	case msg.GetDocumentMessage() != nil:
		m := proto.Clone(msg.GetDocumentMessage()).(*waE2E.DocumentMessage)
		m.JPEGThumbnail = nil
		kind, mimeType = MediaKindDocument, m.GetMimetype()
		fileName = m.GetFileName()
		if fileName == "" {
			fileName = m.GetTitle()
		}
		minimal = &waE2E.Message{DocumentMessage: m}
	case msg.GetStickerMessage() != nil:
		m := proto.Clone(msg.GetStickerMessage()).(*waE2E.StickerMessage)
		m.PngThumbnail = nil
		kind, mimeType = MediaKindSticker, m.GetMimetype()
		minimal = &waE2E.Message{StickerMessage: m}
	default:
		return "", "", "", nil, nil
	}
	raw, err = proto.Marshal(minimal)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("serialize media metadata: %w", err)
	}
	return kind, mimeType, fileName, raw, nil
}

// attachMediaInfo fills the media fields of msg from a whatsmeow message.
func attachMediaInfo(msg *Message, waMsg *waE2E.Message) error {
	kind, mimeType, fileName, raw, err := extractMediaInfo(waMsg)
	if err != nil {
		return err
	}
	msg.MediaKind, msg.MimeType, msg.FileName, msg.MediaProto = kind, mimeType, fileName, raw
	return nil
}

// downloadableFromMessage decodes the stored media proto into the sub-message
// whatsmeow needs for Download/DownloadToFile.
func downloadableFromMessage(msg Message) (whatsmeow.DownloadableMessage, error) {
	if msg.MediaKind == "" || len(msg.MediaProto) == 0 {
		return nil, ErrNotDownloadable
	}
	var wa waE2E.Message
	if err := proto.Unmarshal(msg.MediaProto, &wa); err != nil {
		return nil, fmt.Errorf("decode media metadata: %w", err)
	}
	var d whatsmeow.DownloadableMessage
	switch msg.MediaKind {
	case MediaKindImage:
		if m := wa.GetImageMessage(); m != nil {
			d = m
		}
	case MediaKindVideo:
		if m := wa.GetVideoMessage(); m != nil {
			d = m
		}
	case MediaKindAudio:
		if m := wa.GetAudioMessage(); m != nil {
			d = m
		}
	case MediaKindDocument:
		if m := wa.GetDocumentMessage(); m != nil {
			d = m
		}
	case MediaKindSticker:
		if m := wa.GetStickerMessage(); m != nil {
			d = m
		}
	}
	if d == nil || d.GetDirectPath() == "" || len(d.GetMediaKey()) == 0 {
		return nil, ErrNotDownloadable
	}
	return d, nil
}

// sanitizeFileName reduces an untrusted file name (from a remote sender) to a
// single path element that is also valid and inert on Windows. Backslashes are
// treated as separators so Windows-style names cannot smuggle directories.
// Characters Windows forbids (<>:"|?* and control characters; ':' would also
// select an NTFS alternate data stream) become '_', trailing dots and spaces
// (silently dropped by Windows) are removed, and reserved device names (CON,
// PRN, AUX, NUL, COM0-9, LPT0-9, CONIN$, CONOUT$, with or without extension)
// get a '_' prefix. The same rules apply on every OS so behaviour is uniform.
// Returns ok=false when nothing usable is left.
func sanitizeFileName(name string) (string, bool) {
	name = strings.ReplaceAll(name, "\x00", "")
	name = strings.ReplaceAll(name, "\\", "/")
	base := path.Base(strings.TrimSpace(name))
	if base == "" || base == "." || base == ".." || base == "/" {
		return "", false
	}
	base = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, base)
	base = strings.TrimRight(base, ". ")
	base = strings.TrimLeft(base, " ")
	if base == "" {
		return "", false
	}
	if isWindowsReservedName(base) {
		base = "_" + base
	}
	return base, true
}

// isWindowsReservedName reports whether name's stem (the part before the first
// dot, trailing spaces ignored) is a Windows reserved device name.
func isWindowsReservedName(name string) bool {
	stem := name
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	stem = strings.ToUpper(strings.TrimRight(stem, " "))
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) {
		c := stem[3]
		return c >= '0' && c <= '9'
	}
	// Windows also reserves COM and LPT with superscript digits 1-3.
	for _, p := range []string{"COM", "LPT"} {
		if strings.HasPrefix(stem, p) {
			switch stem[len(p):] {
			case "\u00b9", "\u00b2", "\u00b3":
				return true
			}
		}
	}
	return false
}

// openableExtensions lists file types handed to the system opener by `open`:
// images, video, audio, PDF and plain text. Anything else (executables,
// scripts, shortcuts, archives, office files with macros, HTML, SVG) is saved
// but not opened, because the opener picks the program by extension and on
// Windows runs executables directly (REVIEW-3 finding 2).
var openableExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true, ".heic": true, ".heif": true,
	".mp4": true, ".m4v": true, ".mov": true, ".webm": true, ".mkv": true, ".3gp": true, ".avi": true,
	".mp3": true, ".m4a": true, ".aac": true, ".ogg": true, ".oga": true, ".opus": true, ".wav": true, ".flac": true, ".amr": true,
	".pdf": true,
	".txt": true,
}

// safeToOpen reports whether a downloaded file may be passed to the system
// opener, judged by its final on-disk extension (case-insensitive).
func safeToOpen(filePath string) bool {
	return openableExtensions[strings.ToLower(filepath.Ext(filePath))]
}

// openRefusalReason explains why safeToOpen refused filePath.
func openRefusalReason(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	const allowed = "only images, video, audio, PDF and plain text are opened automatically."
	if ext == "" {
		return "the file has no extension; " + allowed
	}
	return ext + " files can run code or macros; " + allowed
}

// downloadFileName chooses the on-disk name for a message's media: the
// sanitized original name if present, otherwise the message ID plus an
// extension derived from the mimetype.
func downloadFileName(msg Message) string {
	if name, ok := sanitizeFileName(msg.FileName); ok {
		return name
	}
	ext := ""
	if msg.MimeType != "" {
		mt := msg.MimeType
		if i := strings.Index(mt, ";"); i >= 0 {
			mt = strings.TrimSpace(mt[:i])
		}
		if exts, err := mime.ExtensionsByType(mt); err == nil && len(exts) > 0 {
			ext = exts[0]
		}
	}
	if id, ok := sanitizeFileName(msg.Id); ok {
		return id + ext
	}
	return "download" + ext
}

// safeJoin joins dir and name and verifies the result stays inside dir.
func safeJoin(dir, name string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve directory %q: %w", dir, err)
	}
	full := filepath.Join(absDir, name)
	rel, err := filepath.Rel(absDir, full)
	if err != nil {
		return "", fmt.Errorf("check path %q: %w", full, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("file name %q escapes target directory", name)
	}
	return full, nil
}

// urlPattern matches http(s) URLs in message text (same as upstream).
var urlPattern = regexp.MustCompile(`https?://[^\s]+`)

// firstURL returns the first http(s) URL in text, with common trailing
// punctuation removed, or "" when none is found or it does not parse.
func firstURL(text string) string {
	raw := urlPattern.FindString(text)
	raw = strings.TrimRight(raw, ".,;:!?'\"")
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return raw
}
