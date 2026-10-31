// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/PalWorks/whatscli/config"
	"github.com/PalWorks/whatscli/messages"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// get a string representation of all messages for chat
func getMessagesString(msgs []messages.Message) string {
	out := ""
	for _, msg := range msgs {
		if strings.TrimSpace(msg.Text) == "" {
			continue // skip empty/protocol messages
		}
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
		out += "[-::d](" + time + ") [" + colorContact + "::b]" + tview.Escape(msg.ContactShort) + ": [-::-]" + text
	}
	out += "[\"\"]"
	return out
}

// formatRelativeTime formats a Unix timestamp into a WhatsApp Web–style
// relative label: "just now", "5m", "2h", "Yesterday", weekday, or "Jan 02".
func formatRelativeTime(timestamp int64) string {
	if timestamp <= 0 {
		return ""
	}
	now := time.Now()
	t := time.Unix(timestamp, 0)
	diff := now.Sub(t)

	// Guard against future timestamps (clock skew / server-side drift)
	if diff < 0 {
		return "just now"
	}

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	case diff < 24*time.Hour:
		// Same calendar day check
		yN, mN, dN := now.Date()
		yT, mT, dT := t.Date()
		if yN == yT && mN == mT && dN == dT {
			return fmt.Sprintf("%dh", int(diff.Hours()))
		}
		return "Yesterday"
	case diff < 48*time.Hour:
		return "Yesterday"
	case diff < 7*24*time.Hour:
		return t.Weekday().String()[:3] // "Mon", "Tue", etc.
	default:
		// Same year → "Jan 02"; an earlier year → "Jan '06" (short, so
		// chat names keep more room in the list)
		if t.Year() == now.Year() {
			return t.Format("Jan 02")
		}
		return t.Format("Jan '06")
	}
}

// chatTimeText is the right-hand cell of a chat list row: the unread count,
// if any, then the time of the last message.
func chatTimeText(conv *messages.Conversation) string {
	ts := formatRelativeTime(conv.LastMsgTime)
	if conv.Unread == 0 {
		return ts
	}
	unread := "([" + config.Config.Colors.UnreadCount + "]" + fmt.Sprint(conv.Unread) + "[-])"
	if ts == "" {
		return unread
	}
	return unread + " " + ts
}

// RenderChatTable renders the status, chat and group tables from ctx.AllChats
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

	// The time column (with the unread count) must stay visible, so names
	// are cut to the width left over by the widest time cell.
	tsWidth := 0
	for _, conv := range displayList {
		if w := tview.TaggedStringWidth(chatTimeText(conv)); w > tsWidth {
			tsWidth = w
		}
	}
	// Sidebar width minus the grid and table borders, the time and a space.
	nameWidth := config.Config.Ui.ChatSidebarWidth - 4 - tsWidth - 1
	if nameWidth < 8 {
		nameWidth = 8
	}

	// Keep each list where the user scrolled it. Setting the offset also
	// stops tview from following the end of a table that was empty when it
	// was first drawn, which showed the oldest chats at startup.
	offS, _ := ctx.StatusTable.GetOffset()
	offC, _ := ctx.ChatTable.GetOffset()
	offG, _ := ctx.GroupTable.GetOffset()
	defer func() {
		ctx.StatusTable.SetOffset(offS, 0)
		ctx.ChatTable.SetOffset(offC, 0)
		ctx.GroupTable.SetOffset(offG, 0)
	}()

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
		// Skip archived chats
		if conv.IsArchived {
			continue
		}

		// Name cell
		name := conv.Name
		if name == "" {
			name = conv.JID // Fallback
		}

		// Pinned indicator
		if conv.IsPinned {
			name = "📌 " + name
		}

		cell := tview.NewTableCell(name).
			SetReference(conv).
			SetExpansion(1).
			SetMaxWidth(nameWidth).
			SetSelectable(true)

		// Time cell (right-aligned, dim) with the unread count in front
		tsCell := tview.NewTableCell(chatTimeText(conv)).
			SetAlign(tview.AlignRight).
			SetSelectable(true).
			SetExpansion(0)

		// Color & Split
		if conv.JID == messages.STATUSSUFFIX {
			// Status
			cell.SetTextColor(tcell.ColorYellow) // Distinct color
			tsCell.SetTextColor(tcell.ColorDimGray)
			ctx.StatusTable.SetCell(sIdx, 0, cell)
			ctx.StatusTable.SetCell(sIdx, 1, tsCell)
			if conv.JID == selectedStatusJID {
				newRowS = sIdx
			}
			sIdx++
		} else if strings.HasSuffix(conv.JID, messages.GROUPSUFFIX) {
			// Group
			cell.SetTextColor(tcell.ColorNames[config.Config.Colors.ListGroup])
			tsCell.SetTextColor(tcell.ColorDimGray)
			ctx.GroupTable.SetCell(gIdx, 0, cell)
			ctx.GroupTable.SetCell(gIdx, 1, tsCell)
			if conv.JID == selectedGroupJID {
				newRowG = gIdx
			}
			gIdx++
		} else {
			// Contact
			cell.SetTextColor(tcell.ColorNames[config.Config.Colors.ListContact])
			tsCell.SetTextColor(tcell.ColorDimGray)
			ctx.ChatTable.SetCell(cIdx, 0, cell)
			ctx.ChatTable.SetCell(cIdx, 1, tsCell)
			if conv.JID == selectedChatJID {
				newRowC = cIdx
			}
			cIdx++
		}
	}

	// Restore selection — guard prevents SetSelectionChangedFunc from
	// firing SetDisplayedChat during this programmatic Select().
	ctx.RenderingList = true
	if ctx.StatusTable.GetRowCount() > 0 {
		ctx.StatusTable.Select(newRowS, 0)
	}

	if ctx.ChatTable.GetRowCount() > 0 {
		ctx.ChatTable.Select(newRowC, 0)
	}

	if ctx.GroupTable.GetRowCount() > 0 {
		ctx.GroupTable.Select(newRowG, 0)
	}
	ctx.RenderingList = false
}

// imageViewerTimeout bounds one run of show_command, so a hung viewer is
// killed instead of leaving the image request pending forever.
const imageViewerTimeout = 15 * time.Second

// renderImage runs the image viewer showCommand on path and returns what it
// writes to standard output (ANSI text). It blocks until the viewer exits or
// timeout passes, so it must run off the UI goroutine (ROADMAP row 6); the
// process is always waited for, so no zombie is left.
//
// Only files strictly inside one of dirs (the configured download and
// preview directories) are shown. Both sides are symlink-resolved and
// containment is checked with filepath.Rel (audit S-2, REVIEW-3 finding 4).
func renderImage(showCommand, path string, timeout time.Duration, dirs ...string) ([]byte, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("file not found: %v", err)
	}
	resolvedPath, err := messages.ResolveFileWithin(path, dirs...)
	if err != nil {
		return nil, err
	}

	cmdParts := strings.Fields(showCommand)
	if len(cmdParts) == 0 {
		return nil, errors.New("show_command is not configured")
	}
	cmdParts = append(cmdParts, resolvedPath)

	runCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, cmdParts[0], cmdParts[1:]...)
	// If the viewer leaves a child holding stdout open, stop waiting for it
	// shortly after the viewer itself was killed.
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if runCtx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("image viewer %s timed out after %s", cmdParts[0], timeout)
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			msg := strings.TrimSpace(string(exitErr.Stderr))
			if len(msg) > 200 {
				msg = msg[:200] + "..."
			}
			return nil, fmt.Errorf("image viewer %s: %w: %s", cmdParts[0], err, msg)
		}
		return nil, fmt.Errorf("image viewer %s: %w", cmdParts[0], err)
	}
	return out, nil
}
