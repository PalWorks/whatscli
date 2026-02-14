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
// TODO: optimize, use Sprintf etc
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

// RenderChatTable renders the table based on allChats and chatLimit
func RenderChatTable() {
	// Snapshot current selection by JID
	var selectedStatusJID string
	var selectedChatJID string
	var selectedGroupJID string

	if rowS, _ := statusTable.GetSelection(); rowS >= 0 && rowS < statusTable.GetRowCount() {
		if cell := statusTable.GetCell(rowS, 0); cell != nil {
			if ref := cell.GetReference(); ref != nil {
				selectedStatusJID = ref.(*messages.Conversation).JID
			}
		}
	}
	if rowC, _ := chatTable.GetSelection(); rowC >= 0 && rowC < chatTable.GetRowCount() {
		if cell := chatTable.GetCell(rowC, 0); cell != nil {
			if ref := cell.GetReference(); ref != nil {
				selectedChatJID = ref.(*messages.Conversation).JID
			}
		}
	}
	if rowG, _ := groupTable.GetSelection(); rowG >= 0 && rowG < groupTable.GetRowCount() {
		if cell := groupTable.GetCell(rowG, 0); cell != nil {
			if ref := cell.GetReference(); ref != nil {
				selectedGroupJID = ref.(*messages.Conversation).JID
			}
		}
	}

	displayList := allChats
	if len(displayList) > chatLimit {
		displayList = displayList[:chatLimit]
	}

	statusTable.Clear()
	chatTable.Clear()
	groupTable.Clear()

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
			statusTable.SetCell(sIdx, 0, cell)
			if conv.JID == selectedStatusJID {
				newRowS = sIdx
			}
			sIdx++
		} else if strings.HasSuffix(conv.JID, messages.GROUPSUFFIX) {
			// Group
			cell.SetTextColor(tcell.ColorNames[config.Config.Colors.ListGroup])
			groupTable.SetCell(gIdx, 0, cell)
			if conv.JID == selectedGroupJID {
				newRowG = gIdx
			}
			gIdx++
		} else {
			// Contact
			cell.SetTextColor(tcell.ColorNames[config.Config.Colors.ListContact])
			chatTable.SetCell(cIdx, 0, cell)
			if conv.JID == selectedChatJID {
				newRowC = cIdx
			}
			cIdx++
		}
	}

	// Restore selection
	if statusTable.GetRowCount() > 0 {
		statusTable.Select(newRowS, 0)
	}

	if chatTable.GetRowCount() > 0 {
		chatTable.Select(newRowC, 0)
	}

	if groupTable.GetRowCount() > 0 {
		groupTable.Select(newRowG, 0)
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
			io.Copy(tview.ANSIWriter(textView), reader)
			return
		}
	}
	PrintError(err)
}
