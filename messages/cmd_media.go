package messages

import (
	"context"
	"fmt"
	"os"
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

		if client != nil {
			data, err := os.ReadFile(filePath)
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("failed to read file: %v", err))
				return
			}

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
						Mimetype:      proto.String("image/jpeg"),
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
						Mimetype:      proto.String("video/mp4"),
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
						Mimetype:      proto.String("audio/ogg; codecs=opus"),
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
						Mimetype:      proto.String("application/octet-stream"),
						FileEncSHA256: uploaded.FileEncSHA256,
						FileSHA256:    uploaded.FileSHA256,
						FileLength:    proto.Uint64(uint64(len(data))),
						FileName:      proto.String(filePath),
					},
				}
			}

			resp, err := client.SendMessage(context.Background(), jid, msg)
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("failed to send media: %v", err))
			} else {
				sm.uiHandler.PrintText("Media sent: " + resp.ID)
			}
		}
	} else {
		sm.printCommandUsage(cmdName, "<filepath>")
	}
}
