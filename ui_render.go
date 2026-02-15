package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/normen/whatscli/config"
	"github.com/normen/whatscli/messages"
	"github.com/rivo/tview"
)

// get a string representation of all messages for chat
func getMessagesString(msgs []messages.Message) string {
	out := ""
	for _, msg := range msgs {
		out += getTextMessageString(&msg)
		out += "\n"
	}
	return out
}

// create a formatted string with regions based on message ID from a text message
func getTextMessageString(msg *messages.Message) string {
	colorMe := config.Config.Colors.ChatMe
	colorContact := config.Config.Colors.ChatContact
	out := ""
	text := tview.Escape(msg.Text)
	if msg.Forwarded {
		text = "[" + config.Config.Colors.ForwardedText + "]" + text + "[-]"
	}
	tim := time.Unix(int64(msg.Timestamp), 0)
	time := tim.Format("02-01-06 15:04:05")
	out += "[\""
	out += msg.Id
	out += "\"]"
	if msg.FromMe { //msg from me
		out += "[-::d](" + time + ") [" + colorMe + "::b]Me: [-::-]" + text
	} else { // message from others
		out += "[-::d](" + time + ") [" + colorContact + "::b]" + msg.ContactShort + ": [-::-]" + text
	}
	out += "[\"\"]"
	return out
}

// RenderChatTable renders the table based on ctx.AllChats and ctx.ChatLimit
func RenderChatTable() {
	// Snapshot current selection by JID
	var selectedStatusJID string
	var selectedChatJID string
	var selectedGroupJID string

	if rowS, _ := ctx.StatusTable.GetSelection(); rowS >= 0 && rowS < ctx.StatusTable.GetRowCount() {
		if cell := ctx.StatusTable.GetCell(rowS, 0); cell != nil {
			if ref := cell.GetReference(); ref != nil {
				selectedStatusJID = ref.(*messages.Conversation).JID
			}
		}
	}
	if rowC, _ := ctx.ChatTable.GetSelection(); rowC >= 0 && rowC < ctx.ChatTable.GetRowCount() {
		if cell := ctx.ChatTable.GetCell(rowC, 0); cell != nil {
			if ref := cell.GetReference(); ref != nil {
				selectedChatJID = ref.(*messages.Conversation).JID
			}
		}
	}
	if rowG, _ := ctx.GroupTable.GetSelection(); rowG >= 0 && rowG < ctx.GroupTable.GetRowCount() {
		if cell := ctx.GroupTable.GetCell(rowG, 0); cell != nil {
			if ref := cell.GetReference(); ref != nil {
				selectedGroupJID = ref.(*messages.Conversation).JID
			}
		}
	}

	displayList := ctx.AllChats
	if len(displayList) > ctx.ChatLimit {
		displayList = displayList[:ctx.ChatLimit]
	}

	ctx.StatusTable.Clear()
	ctx.ChatTable.Clear()
	ctx.GroupTable.Clear()

	sIdx := 0
	cIdx := 0
	gIdx := 0

	// Indexes to restore
	newRowS := 0
	newRowC := 0
	newRowG := 0

	for _, conv := range displayList {
		// Name cell
		name := conv.Name
		if name == "" {
			name = conv.JID // Fallback
		}

		// Unread status
		if conv.Unread > 0 {
			name += " ([" + config.Config.Colors.UnreadCount + "]" + fmt.Sprint(conv.Unread) + "[-])"
		}

		// Pinned indicator
		if conv.IsPinned {
			name = "📌 " + name
		}

		cell := tview.NewTableCell(name).
			SetReference(conv).
			SetExpansion(1).
			SetSelectable(true)

		// Color & Split
		if conv.JID == messages.STATUSSUFFIX {
			// Status
			cell.SetTextColor(tcell.ColorYellow) // Distinct color
			ctx.StatusTable.SetCell(sIdx, 0, cell)
			if conv.JID == selectedStatusJID {
				newRowS = sIdx
			}
			sIdx++
		} else if strings.HasSuffix(conv.JID, messages.GROUPSUFFIX) {
			// Group
			cell.SetTextColor(tcell.ColorNames[config.Config.Colors.ListGroup])
			ctx.GroupTable.SetCell(gIdx, 0, cell)
			if conv.JID == selectedGroupJID {
				newRowG = gIdx
			}
			gIdx++
		} else {
			// Contact
			cell.SetTextColor(tcell.ColorNames[config.Config.Colors.ListContact])
			ctx.ChatTable.SetCell(cIdx, 0, cell)
			if conv.JID == selectedChatJID {
				newRowC = cIdx
			}
			cIdx++
		}
	}

	// Restore selection
	if ctx.StatusTable.GetRowCount() > 0 {
		ctx.StatusTable.Select(newRowS, 0)
	}

	if ctx.ChatTable.GetRowCount() > 0 {
		ctx.ChatTable.Select(newRowC, 0)
	}

	if ctx.GroupTable.GetRowCount() > 0 {
		ctx.GroupTable.Select(newRowG, 0)
	}
}

// prints an image attachment to the TextView (by message id)
func PrintImage(path string) {
	// Sanitize path to prevent command injection / path traversal
	cleanPath := filepath.Clean(path)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		PrintError(fmt.Errorf("invalid file path: %v", err))
		return
	}

	// Verify the file exists
	if _, err := os.Stat(absPath); err != nil {
		PrintError(fmt.Errorf("file not found: %v", err))
		return
	}

	// Verify the path is under the configured download or preview directory
	downloadDir := filepath.Clean(config.Config.General.DownloadPath)
	previewDir := filepath.Clean(config.Config.General.PreviewPath)
	if !strings.HasPrefix(absPath, downloadDir) && !strings.HasPrefix(absPath, previewDir) {
		PrintError(fmt.Errorf("file path outside allowed directories"))
		return
	}

	cmdParts := strings.Split(config.Config.General.ShowCommand, " ")
	cmdParts = append(cmdParts, absPath)
	var cmd *exec.Cmd
	size := len(cmdParts)
	if size > 1 {
		cmd = exec.Command(cmdParts[0], cmdParts[1:]...)
	} else if size > 0 {
		cmd = exec.Command(cmdParts[0])
	}
	var stdout io.ReadCloser
	if stdout, err = cmd.StdoutPipe(); err == nil {
		if err = cmd.Start(); err == nil {
			reader := bufio.NewReader(stdout)
			io.Copy(tview.ANSIWriter(ctx.TextView), reader)
			return
		}
	}
	PrintError(err)
}
