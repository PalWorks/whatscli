// SPDX-License-Identifier: AGPL-3.0-only

package messages

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PalWorks/whatscli/config"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func cmdSend(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if checkParam(params, 2) {
		receiver := params[0]
		textParams := params[1:]
		text := strings.Join(textParams, " ")
		sm.sendText(receiver, text)
	} else {
		sm.printCommandUsage(cmdName, "[chat-id] [message text]")
	}
}

func cmdSelect(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if checkParam(params, 1) {
		sm.setCurrentReceiver(params[0])
	} else {
		sm.printCommandUsage(cmdName, "[chat-id]")
	}
}

// backlogWait and backlogPoll bound how long cmdBacklog's background
// goroutine polls SQLite for ON_DEMAND history sync results. Variables so
// tests can shorten them.
var (
	backlogWait = 5 * time.Second
	backlogPoll = 250 * time.Millisecond
)

// defaultBacklogCount is used when BacklogMsgQuantity is not positive.
// whatsmeow recommends 50 per request.
const defaultBacklogCount = 50

// cmdBacklog asks the primary phone for older messages of the current chat
// (upstream 1715a36 approach): anchor on the oldest stored message, build an
// ON_DEMAND history sync request and send it as a peer message. The response
// arrives as events.HistorySync and is stored by processHistorySync, which
// also refreshes the open chat. The 5s wait runs in a goroutine so the
// runManager event loop is never blocked.
func cmdBacklog(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.mu.RLock()
	receiver := sm.currentReceiver
	sm.mu.RUnlock()

	if receiver == "" {
		sm.printCommandUsage(cmdName, "-> only works in a chat")
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected to WhatsApp"))
		return
	}

	// Replies are stored under the canonical chat JID (jid_norm.go). An @lid
	// chat stored before its phone-number mapping was known would never see
	// them, so point the user at the chat where they will land.
	if canonical, moved := sm.backlogTarget(receiver); moved {
		sm.uiHandler.PrintText("This chat is stored under its LID form; new and older messages now go to " +
			canonical + ". Open that chat and run " + config.Config.General.CmdPrefix + "backlog there.")
		return
	}

	anchor, ok, err := sm.backlogAnchor(receiver)
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}
	if !ok {
		sm.uiHandler.PrintText("No local message anchor found yet. Open the chat after WhatsApp sync delivers some history, then try " +
			config.Config.General.CmdPrefix + "backlog again.")
		return
	}

	before, err := sm.db.CountMessages(receiver)
	if err != nil {
		sm.uiHandler.PrintError(err)
		return
	}

	count := config.Config.General.BacklogMsgQuantity
	if count <= 0 {
		count = defaultBacklogCount
	}
	sm.uiHandler.PrintText(fmt.Sprintf("Requesting %d messages before %s (%s)...",
		count, anchor.ID, anchor.Timestamp.Format(time.RFC822)))

	req := client.BuildHistorySyncRequest(anchor, count)
	go func() {
		// Network send and polling both run off the event loop.
		client := sm.getClient()
		if client == nil {
			sm.uiHandler.PrintError(errors.New("not connected to WhatsApp"))
			return
		}
		ctx := sm.rootContext()
		if _, err := client.SendPeerMessage(ctx, req); err != nil {
			if ctx.Err() != nil {
				return // shutting down
			}
			sm.uiHandler.PrintError(fmt.Errorf("failed to request message history: %w", err))
			return
		}

		after, err := sm.waitForNewMessages(ctx, receiver, before, backlogWait, backlogPoll)
		if ctx.Err() != nil {
			return // shutting down
		}
		if err != nil {
			sm.uiHandler.PrintError(err)
		}
		if after > before {
			sm.uiHandler.PrintText(fmt.Sprintf("Loaded %d additional messages", after-before))
		} else {
			sm.uiHandler.PrintText("Requested older messages from WhatsApp; none arrived yet. They will appear if the phone answers later.")
		}

		// Only refresh if the user is still viewing the same chat.
		sm.mu.RLock()
		stillCurrent := sm.currentReceiver == receiver
		sm.mu.RUnlock()
		if stillCurrent {
			sm.uiHandler.NewScreen(sm.getMessages(receiver))
		}
	}()
}

// backlogTarget returns the canonical JID of chat and whether it differs from
// chat, i.e. whether history for chat would be stored under another chat.
func (sm *SessionManager) backlogTarget(chat string) (string, bool) {
	jid, err := types.ParseJID(chat)
	if err != nil {
		return chat, false
	}
	canonical := sm.canonicalChatJID(jid).String()
	return canonical, canonical != chat
}

// backlogAnchor builds the MessageInfo of the oldest stored message in chat,
// using the chat and sender JIDs exactly as stored (they may be @lid).
func (sm *SessionManager) backlogAnchor(chat string) (*types.MessageInfo, bool, error) {
	oldest, found, err := sm.db.GetOldestMessage(chat)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	chatJID, err := types.ParseJID(oldest.ChatId)
	if err != nil {
		return nil, false, fmt.Errorf("invalid chat JID %q: %w", oldest.ChatId, err)
	}
	senderJID := types.EmptyJID
	if oldest.ContactId != "" {
		if parsed, perr := types.ParseJID(oldest.ContactId); perr == nil {
			senderJID = parsed
		}
	}
	return &types.MessageInfo{
		ID: oldest.Id,
		MessageSource: types.MessageSource{
			Chat:     chatJID,
			Sender:   senderJID,
			IsFromMe: oldest.FromMe,
			IsGroup:  chatJID.Server == types.GroupServer,
		},
		Timestamp: time.Unix(int64(oldest.Timestamp), 0),
	}, true, nil
}

// waitForNewMessages polls the message count of chat until it exceeds before,
// timeout elapses or ctx is cancelled, and returns the last count seen.
func (sm *SessionManager) waitForNewMessages(ctx context.Context, chat string, before int, timeout, interval time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		n, err := sm.db.CountMessages(chat)
		if err != nil {
			return before, err
		}
		if n > before || !time.Now().Before(deadline) {
			return n, nil
		}
		select {
		case <-ctx.Done():
			return n, ctx.Err()
		case <-ticker.C:
		}
	}
}

func cmdSyncGroups(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.PrintText("Fetching joined groups from WhatsApp servers...")
	go func() {
		// Re-fetch client inside the goroutine — the captured parameter
		// may be stale if the connection dropped between dispatch and
		// goroutine start (audit R-3).
		client := sm.getClient()
		if client == nil {
			sm.uiHandler.PrintError(errors.New("not connected"))
			return
		}
		groups, err := client.GetJoinedGroups(context.Background())
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to fetch groups: %v", err))
			return
		}

		// Batch all PQ operations under a single lock
		var toUpsert []Conversation
		sm.mu.Lock()
		for _, group := range groups {
			jid := group.JID.String()
			name := group.Name
			if name == "" {
				name = "Unknown Group"
			}

			if existing := sm.convByJID[jid]; existing != nil {
				if existing.Name != name {
					existing.Name = name
					toUpsert = append(toUpsert, *existing)
				}
			} else {
				newConv := &Conversation{
					JID:         jid,
					Name:        name,
					LastMsgTime: 0,
					Preview:     "Group synced",
					Unread:      0,
					IsPinned:    false,
				}
				heap.Push(&sm.priorityQueue, newConv)
				sm.convByJID[jid] = newConv
				toUpsert = append(toUpsert, *newConv)
			}
		}
		safeList := sm.snapshotPQ()
		sm.mu.Unlock()

		// DB writes outside lock
		for _, c := range toUpsert {
			if err := sm.db.UpsertConversation(c); err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("upsert conversation: %v", err))
			}
		}

		sm.uiHandler.UpdateChatList(safeList)
		sm.uiHandler.PrintText(fmt.Sprintf("Synced %d groups.", len(groups)))
	}()
}
