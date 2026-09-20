package messages

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func cmdMedia(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if checkParam(params, 1) {
		filePath := strings.Join(params, " ")
		sm.mu.RLock()
		receiver := sm.currentReceiver
		sm.mu.RUnlock()

		jid, err := types.ParseJID(receiver)
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("invalid JID: %v", err))
			return
		}

		// Status (Phase 5.6) is a read-only view — refuse to post media into it.
		if receiver == STATUSSUFFIX {
			sm.uiHandler.PrintError(errors.New("cannot send media to Status (view only)"))
			return
		}

		// Stickers must be WhatsApp-compatible WebP; validate before the
		// connectivity check (matching the local-checks-first convention
		// used by cmdDownload/cmdShow/cmdReply/cmdEdit) so a wrong file type
		// fails fast with a clear message instead of a misleading "not
		// connected".
		if cmdName == "sendsticker" && !isWebpFile(filePath) {
			sm.uiHandler.PrintError(errors.New("stickers must be a .webp file"))
			return
		}

		if client != nil {
			// S-3: Check file size before reading to prevent OOM
			const maxMediaSize = 100 * 1024 * 1024 // 100 MB
			fi, err := os.Stat(filePath)
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("failed to stat file: %v", err))
				return
			}
			if fi.Size() > maxMediaSize {
				sm.uiHandler.PrintError(fmt.Errorf("file too large: %d MB (max 100 MB)", fi.Size()/(1024*1024)))
				return
			}

			// Reading a (potentially up to 100MB) file and uploading it over
			// the network must not block the command loop.
			go func() {
				data, err := os.ReadFile(filePath)
				if err != nil {
					sm.uiHandler.PrintError(fmt.Errorf("failed to read file: %v", err))
					return
				}

				respID, err := uploadAndSendMedia(client, jid, cmdName, data, filePath)
				if err != nil {
					sm.uiHandler.PrintError(err)
				} else {
					sm.uiHandler.PrintText("Media sent: " + respID)
				}
			}()
		} else {
			sm.uiHandler.PrintError(errors.New("not connected"))
		}
	} else {
		sm.printCommandUsage(cmdName, "<filepath>")
	}
}

// mediaTypesByCommand maps a command name to the whatsmeow.MediaType key
// needed for client.Upload. Shared by cmdMedia and the voice-recording
// completion path (cmd_record.go), both of which funnel through
// uploadAndSendMedia.
var mediaTypesByCommand = map[string]whatsmeow.MediaType{
	"sendimage":   whatsmeow.MediaImage,
	"sendvideo":   whatsmeow.MediaVideo,
	"sendaudio":   whatsmeow.MediaAudio,
	"upload":      whatsmeow.MediaDocument,
	"sendsticker": whatsmeow.MediaImage,
}

// uploadAndSendMedia uploads data to WhatsApp's media servers and sends it to
// jid as the message type named by cmdName (one of mediaTypesByCommand's
// keys; anything else is sent as a document). filePath is used only to guess
// a MIME type and, for documents, a display filename — it need not be the
// literal source of data (the voice-recording path passes its temp file's
// path here for exactly that reason). Returns the sent message's ID.
func uploadAndSendMedia(client *whatsmeow.Client, jid types.JID, cmdName string, data []byte, filePath string) (string, error) {
	// CQ-4: Auto-detect MIME type from file content and extension. Stickers
	// are always image/webp regardless of what content sniffing or a
	// mismatched extension might suggest — WhatsApp requires it, and
	// cmdMedia has already validated the source as a .webp file.
	detectedMIME := detectMIME(filePath, data)
	if cmdName == "sendsticker" {
		detectedMIME = "image/webp"
	}

	mediaType := mediaTypesByCommand[cmdName]

	// One transfer-tier deadline covers both the upload and the send: the
	// upload dominates, and a stalled link should fail the whole operation
	// rather than hang it (plan H1.1).
	ctx, cancel := netTransferCtx()
	defer cancel()

	uploaded, err := client.Upload(ctx, data, mediaType)
	if err != nil {
		return "", fmt.Errorf("failed to upload: %v", err)
	}

	var msg *waProto.Message
	switch cmdName {
	case "sendimage":
		msg = &waProto.Message{
			ImageMessage: &waProto.ImageMessage{
				URL:           proto.String(uploaded.URL),
				DirectPath:    proto.String(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      proto.String(detectedMIME),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(data))),
			},
		}
	case "sendvideo":
		msg = &waProto.Message{
			VideoMessage: &waProto.VideoMessage{
				URL:           proto.String(uploaded.URL),
				DirectPath:    proto.String(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      proto.String(detectedMIME),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(data))),
			},
		}
	case "sendaudio":
		msg = &waProto.Message{
			AudioMessage: &waProto.AudioMessage{
				URL:           proto.String(uploaded.URL),
				DirectPath:    proto.String(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      proto.String(detectedMIME),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(data))),
				PTT:           proto.Bool(true),
			},
		}
	case "sendsticker":
		msg = &waProto.Message{
			StickerMessage: &waProto.StickerMessage{
				URL:           proto.String(uploaded.URL),
				DirectPath:    proto.String(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      proto.String(detectedMIME),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(data))),
			},
		}
	default:
		msg = &waProto.Message{
			DocumentMessage: &waProto.DocumentMessage{
				URL:           proto.String(uploaded.URL),
				DirectPath:    proto.String(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      proto.String(detectedMIME),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(data))),
				FileName:      proto.String(filepath.Base(filePath)),
			},
		}
	}

	resp, err := client.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", fmt.Errorf("failed to send media: %v", err)
	}
	return resp.ID, nil
}

// isWebpFile reports whether path has a .webp extension (case-insensitive).
// WhatsApp stickers must be WebP; this is a cheap, local, testable guard
// checked before any file I/O or network call.
func isWebpFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".webp")
}

// detectMIME returns the MIME type for a file, preferring extension-based
// detection and falling back to content sniffing (audit CQ-4).
func detectMIME(path string, data []byte) string {
	// 1. Try extension-based detection
	ext := filepath.Ext(path)
	if ext != "" {
		if mt := mime.TypeByExtension(ext); mt != "" {
			return mt
		}
	}
	// 2. Fall back to content sniffing
	if len(data) > 0 {
		return http.DetectContentType(data)
	}
	return "application/octet-stream"
}
