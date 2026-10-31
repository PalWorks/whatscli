// SPDX-License-Identifier: AGPL-3.0-only

package messages

// cmd_message.go — commands that act on a single highlighted message:
// download, open, show, url, revoke. The UI sends the message ID as the only
// parameter (ui_keybindings.go handleMessageCommand).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PalWorks/whatscli/config"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// downloadMode selects what happens after a media file has been saved.
type downloadMode int

const (
	modeDownload downloadMode = iota // save to DownloadPath and print the path
	modeOpen                         // save to PreviewPath and open with the system opener
	modeShow                         // save to PreviewPath and render inline (images only)
)

// mediaDownloader is the subset of *whatsmeow.Client used for downloads,
// so tests can substitute a fake.
type mediaDownloader interface {
	DownloadToFile(ctx context.Context, msg whatsmeow.DownloadableMessage, file whatsmeow.File) error
}

func cmdDownload(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	runDownload(sm, client, cmdName, params, modeDownload)
}

func cmdOpen(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	runDownload(sm, client, cmdName, params, modeOpen)
}

func cmdShow(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	runDownload(sm, client, cmdName, params, modeShow)
}

// lookupMessage loads a message by ID and reports errors to the UI.
func (sm *SessionManager) lookupMessage(id string) (Message, bool) {
	msg, found, err := sm.db.GetMessage(id)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("load message: %w", err))
		return Message{}, false
	}
	if !found {
		sm.uiHandler.PrintError(fmt.Errorf("message %s not found", id))
		return Message{}, false
	}
	return msg, true
}

func runDownload(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string, mode downloadMode) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "[message-id]")
		return
	}
	msg, ok := sm.lookupMessage(params[0])
	if !ok {
		return
	}
	if _, err := downloadableFromMessage(msg); err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	if mode == modeShow && msg.MediaKind != MediaKindImage {
		sm.uiHandler.PrintError(errors.New("show only works for image messages"))
		return
	}
	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected to WhatsApp"))
		return
	}

	dir := config.Config.General.DownloadPath
	if mode != modeDownload {
		dir = config.Config.General.PreviewPath
	}

	sm.uiHandler.PrintText("[::d]downloading " + msg.Id + "...[::-]")
	// Downloads can take a long time; never block runManager's event loop.
	go func() {
		// Re-fetch the client: the captured one may be stale (audit R-3).
		client := sm.getClient()
		if client == nil {
			sm.uiHandler.PrintError(errors.New("not connected to WhatsApp"))
			return
		}
		path, err := downloadMedia(context.Background(), client, msg, dir)
		if err != nil {
			sm.uiHandler.PrintError(err)
			return
		}
		sm.deliverDownload(mode, path)
	}()
}

// deliverDownload hands a saved file to the UI according to mode. `open` only
// passes allow-listed types (safeToOpen) to the system opener; anything else
// is reported with its path so the user decides.
func (sm *SessionManager) deliverDownload(mode downloadMode, path string) {
	switch mode {
	case modeShow:
		sm.uiHandler.PrintFile(path)
	case modeOpen:
		if !safeToOpen(path) {
			// Sender-chosen types such as .exe, .bat or .lnk would run.
			sm.uiHandler.PrintText("[::d] -> " + path + "[::-]")
			sm.uiHandler.PrintText("Saved but not opened: " + openRefusalReason(path) +
				" Open it yourself only if you trust the sender.")
			return
		}
		sm.uiHandler.OpenFile(path)
	default:
		sm.uiHandler.PrintText("[::d] -> " + path + "[::-]")
	}
}

// downloadMedia saves msg's media into dir and returns the final path.
// The file name is sanitized and confined to dir. If a different file already
// uses the name, "<stem>_<message-id><ext>" is used instead; an existing file
// under that ID-specific name is reused without downloading again. Data is
// written to a temporary file first and renamed into place on success.
func downloadMedia(ctx context.Context, dl mediaDownloader, msg Message, dir string) (string, error) {
	d, err := downloadableFromMessage(msg)
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", errors.New("download directory is not configured")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create download directory: %w", err)
	}

	name := downloadFileName(msg)
	full, err := safeJoin(dir, name)
	if err != nil {
		return "", err
	}
	idSpecific := !hasUsableFileName(msg)
	if _, statErr := os.Lstat(full); statErr == nil {
		if !idSpecific {
			ext := filepath.Ext(name)
			idPart, ok := sanitizeFileName(msg.Id)
			if !ok {
				idPart = "dup"
			}
			name = strings.TrimSuffix(name, ext) + "_" + idPart + ext
			if full, err = safeJoin(dir, name); err != nil {
				return "", err
			}
		}
		if fi, statErr := os.Lstat(full); statErr == nil {
			if fi.Mode().IsRegular() {
				return full, nil // already downloaded for this message
			}
			return "", fmt.Errorf("refusing to overwrite non-regular file %s", full)
		}
	}

	tmp, err := os.CreateTemp(filepath.Dir(full), ".whatscli-*.part")
	if err != nil {
		return "", fmt.Errorf("create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if err := dl.DownloadToFile(ctx, d, tmp); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", fmt.Errorf("download media: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("close downloaded file: %w", err)
	}
	if err := os.Rename(tmpName, full); err != nil {
		cleanup()
		return "", fmt.Errorf("move downloaded file into place: %w", err)
	}
	return full, nil
}

// hasUsableFileName reports whether downloadFileName will use the sender's name.
func hasUsableFileName(msg Message) bool {
	_, ok := sanitizeFileName(msg.FileName)
	return ok
}

func cmdURL(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "[message-id]")
		return
	}
	msg, ok := sm.lookupMessage(params[0])
	if !ok {
		return
	}
	link := firstURL(msg.Text)
	if link == "" {
		sm.uiHandler.PrintText("No URL found in message")
		return
	}
	sm.uiHandler.PrintText("[::d]opening " + link + "[::-]")
	sm.uiHandler.OpenFile(link)
}

// ErrRevokeNotOwn is returned when trying to revoke someone else's message.
var ErrRevokeNotOwn = errors.New("you can only revoke (delete for everyone) your own messages")

func cmdRevoke(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "[message-id]")
		return
	}
	msg, ok := sm.lookupMessage(params[0])
	if !ok {
		return
	}
	if !msg.FromMe {
		sm.uiHandler.PrintError(ErrRevokeNotOwn)
		return
	}
	if msg.Text == RevokedText {
		sm.uiHandler.PrintError(errors.New("message is already revoked"))
		return
	}
	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected to WhatsApp"))
		return
	}
	// Use the chat JID exactly as stored (may be @lid, @s.whatsapp.net or @g.us).
	chatJID, err := types.ParseJID(msg.ChatId)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid chat JID %q: %w", msg.ChatId, err))
		return
	}

	go func() {
		client := sm.getClient()
		if client == nil {
			sm.uiHandler.PrintError(errors.New("not connected to WhatsApp"))
			return
		}
		// EmptyJID sender marks the key FromMe, independent of whether our own
		// ID was stored in phone-number or LID form.
		revoke := client.BuildRevoke(chatJID, types.EmptyJID, types.MessageID(msg.Id))
		if _, err := client.SendMessage(context.Background(), chatJID, revoke); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("revoke message: %w", err))
			return
		}
		sm.applyRevoke(msg.ChatId, msg.Id)
		sm.uiHandler.PrintText("revoked: " + msg.Id)
	}()
}

// applyRevoke marks a message revoked in SQLite and refreshes the screen if
// its chat is currently displayed. Used for our own revokes and, after the
// checks in handleIncomingRevoke, for revokes received from others or from our
// other devices. chatID must be the chat the message is stored under.
func (sm *SessionManager) applyRevoke(chatID, msgID string) {
	updated, err := sm.db.MarkMessageRevoked(chatID, msgID)
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	if !updated {
		return
	}
	sm.mu.RLock()
	isCurrent := sm.currentReceiver == chatID
	sm.mu.RUnlock()
	if isCurrent {
		sm.uiHandler.NewScreen(sm.getMessages(chatID))
	}
}
