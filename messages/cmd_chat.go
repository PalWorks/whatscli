package messages

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

func cmdBacklog(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.mu.RLock()
	receiver := sm.currentReceiver
	sm.mu.RUnlock()

	if receiver == "" {
		sm.printCommandUsage(cmdName, "-> only works in a chat")
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	jid, err := types.ParseJID(receiver)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid JID: %v", err))
		return
	}
	_ = jid // used for anchor construction

	sm.uiHandler.PrintText("Requesting message history from WhatsApp servers...")

	// Build anchor from oldest known message (if any)
	var anchor *types.MessageInfo
	msgs, err := sm.db.GetMessages(receiver)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to load messages: %v", err))
		return
	}
	existingCount := len(msgs)

	if existingCount > 0 {
		oldest := msgs[0]
		chatJID, err := types.ParseJID(oldest.ChatId)
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("invalid chat JID %q: %v", oldest.ChatId, err))
			return
		}
		senderJID, err := types.ParseJID(oldest.ContactId)
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("invalid sender JID %q: %v", oldest.ContactId, err))
			return
		}

		anchor = &types.MessageInfo{
			ID: oldest.Id,
			MessageSource: types.MessageSource{
				Chat:     chatJID,
				Sender:   senderJID,
				IsFromMe: oldest.FromMe,
			},
			Timestamp: time.Unix(int64(oldest.Timestamp), 0),
		}
		sm.uiHandler.PrintText(fmt.Sprintf("Requesting 50 messages before: %s (%s)",
			oldest.Id, time.Unix(int64(oldest.Timestamp), 0).Format(time.RFC822)))
	} else {
		sm.uiHandler.PrintText("Requesting latest 50 messages...")
	}

	req := client.BuildHistorySyncRequest(anchor, 50)

	// Send to primary device (self)
	target := *client.Store.ID
	target.Device = 0

	resp, err := client.SendMessage(context.Background(), target, req)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to send history sync request: %v", err))
		return
	}

	sm.uiHandler.PrintText("History sync request sent (ID: " + resp.ID + "). Waiting for response...")

	// History sync arrives asynchronously; check in background so the
	// command loop is not blocked (audit finding P-1).
	go func() {
		time.Sleep(5 * time.Second)

		finalMessages, err := sm.db.GetMessages(receiver)
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to reload messages: %v", err))
		} else if len(finalMessages) > existingCount {
			sm.uiHandler.PrintText(fmt.Sprintf("Loaded %d additional messages", len(finalMessages)-existingCount))
		} else {
			sm.uiHandler.PrintText("No immediate history received. Messages may arrive in the background.")
		}

		// Refresh screen with whatever we have
		screen := sm.getMessages(receiver)
		sm.uiHandler.NewScreen(screen)
	}()
}

func cmdSyncGroups(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.PrintText("Fetching joined groups from WhatsApp servers...")
	go func() {
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
