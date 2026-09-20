package messages

import (
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/normen/whatscli/config"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// mediaTypeToWhatsmeow maps our stored Message.MediaType string to the
// whatsmeow.MediaType key needed for DownloadMediaWithPath. whatsmeow itself
// classifies stickers as MediaImage internally (see its classToMediaType
// table), so we mirror that here.
var mediaTypeToWhatsmeow = map[string]whatsmeow.MediaType{
	"image":    whatsmeow.MediaImage,
	"video":    whatsmeow.MediaVideo,
	"audio":    whatsmeow.MediaAudio,
	"document": whatsmeow.MediaDocument,
	"sticker":  whatsmeow.MediaImage,
}

// downloadMessage downloads the attachment for a stored message and saves it
// under DownloadPath, returning the path it was written to. The caller is
// responsible for checking msg.MediaType != "" before calling this.
func downloadMessage(client *whatsmeow.Client, msg Message) (string, error) {
	return downloadMessageTo(client, msg, config.Config.General.DownloadPath)
}

// downloadMessageTo downloads the attachment for a stored message into dir,
// returning the path it was written to. If a file with the message's
// expected name already exists in dir, it is reused as-is rather than
// re-downloaded — repeated /download or /show calls on the same message
// don't re-fetch from the network or pile up numbered duplicates. The
// caller is responsible for checking msg.MediaType != "" before calling this.
func downloadMessageTo(client *whatsmeow.Client, msg Message, dir string) (string, error) {
	name := mediaFileName(msg)
	expectedPath := filepath.Join(dir, name)
	if _, err := os.Stat(expectedPath); err == nil {
		return expectedPath, nil
	}

	waType, ok := mediaTypeToWhatsmeow[msg.MediaType]
	if !ok {
		return "", fmt.Errorf("unsupported media type %q", msg.MediaType)
	}

	ctx, cancel := netTransferCtx()
	defer cancel()
	// mmsType left empty: DownloadMediaWithPath resolves it from mediaType internally.
	data, err := client.DownloadMediaWithPath(ctx, msg.DirectPath, msg.FileEncSHA256, msg.FileSHA256, msg.MediaKey, waType, "", false)
	if err != nil {
		return "", fmt.Errorf("failed to download media: %w", err)
	}

	path, err := writeUniqueFile(dir, name, data)
	if err != nil {
		return "", fmt.Errorf("failed to save file: %w", err)
	}
	return path, nil
}

// chatIdSanitizer strips every character from a chat JID that writeUniqueFile
// would otherwise treat as meaningful in a filename: "@" (user/server
// separator) and "." (e.g. the dots in "s.whatsapp.net"). Without this, a
// synthesized name with no real extension (unrecognized mimetype) could
// still contain a dot from the JID, and filepath.Ext would mistake a
// fragment of the JID for a file extension when writeUniqueFile computes a
// collision suffix.
var chatIdSanitizer = strings.NewReplacer("@", "_", ".", "_")

// mediaFileName picks a filename for a downloaded attachment. For documents it
// keeps the sender's original name (nice for the user) but with two mandatory
// guards, because that name is fully attacker-controlled:
//
//  1. Security: it is reduced to filepath.Base so a hostile FileName like
//     "../../../home/user/.bashrc" cannot make the download escape DownloadPath
//     when joined with the target directory. Degenerate bases (".", "..", "/")
//     fall through to the synthesized name.
//  2. Correctness: it is prefixed with the message ID so two different
//     documents that happen to share a name produce different filenames. The
//     reuse-if-exists check in downloadMessageTo keys on this name; without the
//     ID prefix, downloading document B would silently return document A's file.
//
// Non-document attachments use a synthesized <chat>_<id><ext> name (extension
// guessed from the stored mimetype), which is already unique via the ID.
func mediaFileName(msg Message) string {
	if msg.FileName != "" {
		base := filepath.Base(msg.FileName)
		if base != "." && base != ".." && base != string(filepath.Separator) {
			return msg.Id + "_" + base
		}
	}
	safeChatId := chatIdSanitizer.Replace(msg.ChatId)
	return safeChatId + "_" + msg.Id + extensionForMimetype(msg.Mimetype)
}

// extensionForMimetype returns a file extension (including the leading dot)
// for a MIME type, or "" if none is registered.
func extensionForMimetype(mimetype string) string {
	if mimetype == "" {
		return ""
	}
	exts, err := mime.ExtensionsByType(mimetype)
	if err != nil || len(exts) == 0 {
		return ""
	}
	return exts[0]
}

// writeUniqueFile writes data to a file named `name` inside dir without ever
// overwriting an existing file: if the name is taken, an incrementing
// numeric suffix is inserted before the extension until a free name is
// found. Uses O_EXCL so the existence check and creation are atomic.
func writeUniqueFile(dir, name string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create download directory: %w", err)
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)

	for i := 0; ; i++ {
		candidateName := name
		if i > 0 {
			candidateName = fmt.Sprintf("%s_%d%s", base, i, ext)
		}
		path := filepath.Join(dir, candidateName)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", fmt.Errorf("failed to create file: %w", err)
		}
		_, writeErr := f.Write(data)
		closeErr := f.Close()
		if writeErr != nil {
			// Don't leave a partial/zero-byte file behind (e.g. disk full):
			// it would otherwise force the next download to a _1 suffix and
			// masquerade as a real, complete attachment.
			os.Remove(path)
			return "", fmt.Errorf("failed to write file: %w", writeErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("failed to close file: %w", closeErr)
		}
		return path, nil
	}
}

// cmdDownload downloads the attachment on a selected message to DownloadPath.
// Local checks (message lookup, does it have an attachment) run before the
// connectivity check, so a user gets an accurate error instead of a
// misleading "not connected" when the real issue is a bad message ID or a
// text-only message.
func cmdDownload(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "-> select a message first")
		return
	}

	msg, err := sm.db.GetMessageByID(params[0])
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to look up message: %v", err))
		return
	}
	if msg.MediaType == "" {
		sm.uiHandler.PrintError(errors.New("selected message has no attachment"))
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	go func() {
		path, err := downloadMessage(client, msg)
		if err != nil {
			sm.uiHandler.PrintError(err)
			return
		}
		sm.uiHandler.PrintText("Saved: " + path)
	}()
}

// cmdOpen downloads the attachment on a selected message and opens it with
// the OS default handler. See cmdDownload for the guard-ordering rationale.
func cmdOpen(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "-> select a message first")
		return
	}

	msg, err := sm.db.GetMessageByID(params[0])
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to look up message: %v", err))
		return
	}
	if msg.MediaType == "" {
		sm.uiHandler.PrintError(errors.New("selected message has no attachment"))
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	go func() {
		path, err := downloadMessage(client, msg)
		if err != nil {
			sm.uiHandler.PrintError(err)
			return
		}
		sm.uiHandler.PrintText("Saved: " + path)
		sm.uiHandler.OpenFile(path)
	}()
}

// cmdShow downloads the image (or sticker) attachment on a selected message
// into PreviewPath and renders it in-terminal via the configured show_command.
func cmdShow(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "-> select a message first")
		return
	}

	msg, err := sm.db.GetMessageByID(params[0])
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to look up message: %v", err))
		return
	}
	if msg.MediaType != "image" && msg.MediaType != "sticker" {
		sm.uiHandler.PrintError(errors.New("selected message is not an image"))
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	go func() {
		path, err := downloadMessageTo(client, msg, config.Config.General.PreviewPath)
		if err != nil {
			sm.uiHandler.PrintError(err)
			return
		}
		sm.uiHandler.PrintFile(path)
	}()
}

// urlPattern matches a conservative http(s) URL: scheme, then any run of
// non-whitespace characters.
var urlPattern = regexp.MustCompile(`https?://\S+`)

// extractFirstURL returns the first http(s) URL found in text with common
// trailing punctuation stripped (so a URL at the end of a sentence or inside
// parentheses doesn't carry a stray "." or ")" into the opened link), or ""
// if no URL is present.
func extractFirstURL(text string) string {
	match := urlPattern.FindString(text)
	if match == "" {
		return ""
	}
	return strings.TrimRight(match, `.,!?;:)]}'"`)
}

// cmdURL opens the first URL found in a selected message's text using the OS
// default handler (e.g. a web browser).
func cmdURL(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "-> select a message first")
		return
	}

	msg, err := sm.db.GetMessageByID(params[0])
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to look up message: %v", err))
		return
	}

	url := extractFirstURL(msg.Text)
	if url == "" {
		sm.uiHandler.PrintText("No URL in selected message")
		return
	}

	sm.uiHandler.OpenFile(url)
}

// cmdRevoke revokes (deletes for everyone) a selected message that was sent
// by the local user. Revoking other participants' messages requires group
// admin privileges that this command does not attempt to exercise, so it is
// restricted to the local user's own messages.
func cmdRevoke(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "-> select a message first")
		return
	}

	msg, err := sm.db.GetMessageByID(params[0])
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to look up message: %v", err))
		return
	}
	if !msg.FromMe {
		sm.uiHandler.PrintError(errors.New("can only revoke your own messages"))
		return
	}

	chatJID, err := types.ParseJID(msg.ChatId)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid chat JID %q: %v", msg.ChatId, err))
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	go func() {
		revoke := client.BuildRevoke(chatJID, types.EmptyJID, msg.Id)
		ctx, cancel := netCtx()
		defer cancel()
		if _, err := client.SendMessage(ctx, chatJID, revoke); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to revoke message: %v", err))
			return
		}

		if err := sm.db.MarkMessageDeleted(msg.Id); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("revoked on server but failed to update local copy: %v", err))
			return
		}

		sm.mu.RLock()
		isCurrent := sm.currentReceiver == msg.ChatId
		sm.mu.RUnlock()
		if isCurrent {
			sm.uiHandler.NewScreen(sm.getMessages(msg.ChatId))
		}

		sm.uiHandler.PrintText("Message revoked")
	}()
}

// cmdReply sends a quoted reply to a selected message. params[0] is the
// target message ID (normally filled in automatically by the reply
// keybinding, which prefills the input field with "/reply <id> "); the
// remaining params are the reply text. Local/DB checks run before the
// connectivity check, matching the guard-ordering convention established by
// cmdDownload et al.
func cmdReply(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 2) {
		sm.printCommandUsage(cmdName, "-> select a message, then type your reply")
		return
	}

	target, err := sm.db.GetMessageByID(params[0])
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to look up message: %v", err))
		return
	}

	// A real WhatsApp "reply" to a status update is actually sent as a
	// private message to the status author, not to status@broadcast itself
	// — a materially different feature not in this v1's scope (Phase 5.6
	// treats Status as read-only). Refuse rather than send a broken reply
	// to the wrong JID.
	if target.ChatId == STATUSSUFFIX {
		sm.uiHandler.PrintError(errors.New("cannot reply to a status update"))
		return
	}

	text := strings.Join(params[1:], " ")

	chatJID, err := types.ParseJID(target.ChatId)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid chat JID %q: %v", target.ChatId, err))
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	// The network send (and everything depending on its result) must not
	// block the command loop — matches cmdRevoke's convention immediately
	// below, which backgrounds the same kind of client.SendMessage call.
	go func() {
		replyMsg := &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String(text),
				ContextInfo: &waE2E.ContextInfo{
					StanzaID:      proto.String(target.Id),
					Participant:   proto.String(target.ContactId),
					QuotedMessage: &waE2E.Message{Conversation: proto.String(target.Text)},
				},
			},
		}

		ctx, cancel := netCtx()
		defer cancel()
		resp, err := client.SendMessage(ctx, chatJID, replyMsg)
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to send reply: %v", err))
			return
		}

		newMsg := Message{
			Id:            resp.ID,
			ChatId:        target.ChatId,
			FromMe:        true,
			Timestamp:     uint64(time.Now().Unix()),
			Text:          text,
			ContactId:     ownJID(client),
			ContactName:   "Me",
			ContactShort:  "Me",
			QuotedID:      target.Id,
			QuotedPreview: truncatePreview(target.Text),
		}
		if err := sm.db.AddMessage(newMsg); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to save sent reply: %v", err))
		}

		sm.mu.RLock()
		isCurrent := sm.currentReceiver == target.ChatId
		sm.mu.RUnlock()
		if isCurrent {
			sm.uiHandler.NewMessage(newMsg)
		}
	}()
}

// cmdEdit edits a previously sent own message. params[0] is the target
// message ID (normally filled in automatically by the edit keybinding, which
// prefills the input field with "/edit <id> "); the remaining params are the
// new text. Mirrors cmdReply's guard ordering and cmdRevoke's FromMe
// restriction.
func cmdEdit(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 2) {
		sm.printCommandUsage(cmdName, "-> select your own message, then type the new text")
		return
	}

	target, err := sm.db.GetMessageByID(params[0])
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to look up message: %v", err))
		return
	}
	if !target.FromMe {
		sm.uiHandler.PrintError(errors.New("can only edit your own messages"))
		return
	}

	newText := strings.Join(params[1:], " ")

	age := time.Now().Unix() - int64(target.Timestamp)
	if age > int64(whatsmeow.EditWindow.Seconds()) {
		sm.uiHandler.PrintError(fmt.Errorf("edit window (%s) has expired for this message", whatsmeow.EditWindow))
		return
	}

	chatJID, err := types.ParseJID(target.ChatId)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid chat JID %q: %v", target.ChatId, err))
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	// See cmdReply's identical comment: the network send must not block the
	// command loop.
	go func() {
		edited := &waE2E.Message{Conversation: proto.String(newText)}
		built := client.BuildEdit(chatJID, target.Id, edited)

		ctx, cancel := netCtx()
		defer cancel()
		if _, err := client.SendMessage(ctx, chatJID, built); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to send edit: %v", err))
			return
		}

		if err := sm.db.UpdateMessageText(target.Id, newText+" (edited)"); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("edited on server but failed to update local copy: %v", err))
			return
		}

		sm.mu.RLock()
		isCurrent := sm.currentReceiver == target.ChatId
		sm.mu.RUnlock()
		if isCurrent {
			sm.uiHandler.NewScreen(sm.getMessages(target.ChatId))
		}

		sm.uiHandler.PrintText("Message edited")
	}()
}
