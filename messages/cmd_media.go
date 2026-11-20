// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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
		sendMediaFile(sm, client, receiver, cmdName, filePath)
	} else {
		sm.printCommandUsage(cmdName, "<filepath>")
	}
}

// mediaCommands are the commands that send a file; the file picker uses
// the same names to say how a picked file is sent.
var mediaCommands = map[string]bool{"upload": true, "sendimage": true, "sendvideo": true, "sendaudio": true}

// parseSendFile reads "<chat-id> <upload|sendimage|sendvideo|sendaudio>
// <path>" for /sendfile. The path may contain spaces.
func parseSendFile(params []string) (chat, cmd, path string, ok bool) {
	if len(params) < 3 || params[0] == "" || !mediaCommands[params[1]] {
		return "", "", "", false
	}
	path = strings.Join(params[2:], " ")
	if path == "" {
		return "", "", "", false
	}
	return params[0], params[1], path, true
}

// cmdSendFile sends a file to the chat named in the command rather than the
// open one. The file picker uses it so every picked file goes to the chat it
// was picked in, even if the user switches chats while files are uploading.
func cmdSendFile(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	chat, cmd, path, ok := parseSendFile(params)
	if !ok {
		sm.printCommandUsage(cmdName, "<chat-id> <upload|sendimage|sendvideo|sendaudio> <filepath>")
		return
	}
	sendMediaFile(sm, client, chat, cmd, path)
}

// sendMediaFile uploads filePath and sends it to receiver as the kind of
// message cmdName names (upload sends a document).
func sendMediaFile(sm *SessionManager, client *whatsmeow.Client, receiver, cmdName, filePath string) {
	{
		jid, err := types.ParseJID(receiver)
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("invalid JID: %v", err))
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

			data, err := os.ReadFile(filePath)
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("failed to read file: %v", err))
				return
			}

			// CQ-4: Auto-detect MIME type from file content and extension
			detectedMIME := detectMIME(filePath, data)

			// Map command to correct media type for upload
			mediaTypes := map[string]whatsmeow.MediaType{
				"sendimage": whatsmeow.MediaImage,
				"sendvideo": whatsmeow.MediaVideo,
				"sendaudio": whatsmeow.MediaAudio,
				"upload":    whatsmeow.MediaDocument,
			}
			mediaType := mediaTypes[cmdName]

			var msg *waProto.Message
			uploaded, err := client.Upload(context.Background(), data, mediaType)
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("failed to upload: %v", err))
				return
			}

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

			resp, err := client.SendMessage(context.Background(), jid, msg)
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("failed to send media: %v", err))
			} else {
				sm.uiHandler.PrintText("Media sent: " + resp.ID)
				sm.storeSentMedia(client, receiver, resp.ID, resp.Timestamp, msg)
			}
		}
	}
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

// storeSentMedia persists an outgoing media message (including download
// metadata) and shows it in the open chat, mirroring sendText.
func (sm *SessionManager) storeSentMedia(client *whatsmeow.Client, chatID string, id types.MessageID, ts time.Time, waMsg *waProto.Message) {
	text, _ := extractMessageContent(waMsg)
	if ts.IsZero() {
		ts = time.Now()
	}
	contactID := ""
	if client != nil && client.Store != nil && client.Store.ID != nil {
		contactID = client.Store.ID.String()
	}
	newMsg := Message{
		Id:           id,
		ChatId:       chatID,
		FromMe:       true,
		Timestamp:    uint64(ts.Unix()),
		Text:         text,
		ContactId:    contactID,
		ContactName:  "Me",
		ContactShort: "Me",
	}
	if err := attachMediaInfo(&newMsg, waMsg); err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("sent media %s: %w", id, err))
	}
	if _, err := sm.db.AddMessage(newMsg); err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to save sent media: %w", err))
		return
	}
	sm.mu.RLock()
	isCurrent := sm.currentReceiver == chatID
	sm.mu.RUnlock()
	if isCurrent {
		sm.uiHandler.NewMessage(newMsg)
	}
}
