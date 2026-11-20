// SPDX-License-Identifier: AGPL-3.0-only

package main

// ui_picker.go: the file picker. /upload, /sendimage, /sendvideo and
// /sendaudio typed without a path, or the pick_file key (Ctrl+o), hand the
// terminal to an embedded fzf (package picker) and send each chosen file to
// the chat that was open when the picker opened.

import (
	"fmt"
	"strings"

	"github.com/PalWorks/whatscli/config"
	"github.com/PalWorks/whatscli/messages"
	"github.com/PalWorks/whatscli/picker"
	"github.com/gdamore/tcell/v2"
)

// opensPicker reports whether a typed command should open the picker: a
// file-sending command with no path.
func opensPicker(cmd string, params []string) bool {
	switch cmd {
	case "upload", "sendimage", "sendvideo", "sendaudio":
		return len(params) == 0
	}
	return false
}

// pickerHint reports whether text is a file-sending command followed by a
// space and no path yet ("/upload "), and which command. The drop-down then
// shows one line saying Enter opens the search.
func pickerHint(text, prefix string) (string, bool) {
	if prefix == "" || !strings.HasPrefix(text, prefix) || strings.TrimSpace(text) == text {
		return "", false
	}
	fields := strings.Fields(strings.TrimPrefix(text, prefix))
	if len(fields) != 1 {
		return "", false
	}
	cmd := strings.ToLower(fields[0])
	if !opensPicker(cmd, nil) {
		return "", false
	}
	return cmd, true
}

// pickerNouns names what each command's search lists.
var pickerNouns = map[string]string{"upload": "files", "sendimage": "images", "sendvideo": "videos", "sendaudio": "audio files"}

// pickerHintEntry is the drop-down line under "<prefix><cmd> ".
func pickerHintEntry(cmd string) string {
	return fmt.Sprintf("%-28s %s", "Enter", "search "+pickerNouns[cmd]+" with fzf (or type a path)")
}

// pickedCommands turns picked files into one /sendfile command each, bound
// to chatID. With auto, each file's command is chosen from the file itself
// instead of using cmd.
func pickedCommands(paths []string, chatID, cmd string, auto bool, ext picker.Extensions) []messages.Command {
	cmds := make([]messages.Command, 0, len(paths))
	for _, path := range paths {
		c := cmd
		if auto {
			c = ext.CommandFor(path)
		}
		cmds = append(cmds, messages.Command{Name: "sendfile", Params: []string{chatID, c, path}})
	}
	return cmds
}

// openPicker hands the terminal to fzf and sends the picked files to the open
// chat. cmd is the send command (upload, sendimage, ...); with auto every file
// is offered and sent as what it is. UI goroutine only.
func openPicker(cmd string, auto bool) {
	if ctx.CurrentReceiver.Id == "" {
		PrintText("[" + config.Config.Colors.Negative + "]Usage:[-] " + config.Config.General.CmdPrefix + cmd + " needs an open chat")
		return
	}
	chatID := ctx.CurrentReceiver.Id // fixed now: the user may switch chats while files upload
	ext := picker.ExtensionsFromConfig()
	opts := picker.Options{Root: config.Config.General.UploadSearchPath, Prompt: cmd + "> "}
	if auto {
		opts.Prompt = "send> "
	} else {
		opts.Allow = ext.AllowFor(cmd)
	}
	var paths []string
	var err error
	ctx.App.Suspend(func() {
		paths, err = picker.Pick(opts)
	})
	if err != nil {
		PrintError(err)
		return
	}
	cmds := pickedCommands(paths, chatID, cmd, auto, ext)
	// Sent from a goroutine: CommandChannel holds 10 and an upload can take
	// seconds, so the UI goroutine must not wait for room.
	go func() {
		for _, c := range cmds {
			ctx.SessionManager.CommandChannel <- c
		}
	}()
}

// handlePickFile opens the picker over every file; each picked file is sent
// as what it is (image, video, audio, else document).
func handlePickFile(ev *tcell.EventKey) *tcell.EventKey {
	openPicker("upload", true)
	return nil
}
