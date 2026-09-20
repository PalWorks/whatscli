package messages

// cmd_group.go — group management commands. Every handler follows the same
// shape (plan H1.4): cheap local guards (params, JID parsing) run
// synchronously so tests and users get accurate errors, the connectivity
// check runs synchronously, and the network call plus its result reporting
// run in a goroutine with a bounded context so a slow link can neither block
// the command loop nor hang forever.

import (
	"errors"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func cmdCreate(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 2) {
		sm.printCommandUsage(cmdName, "<phone1,phone2> <subject>")
		return
	}

	// /create <phones> <subject>
	// phones is a comma-separated list
	phones := strings.Split(params[0], ",")
	subject := strings.Join(params[1:], " ")

	participants := []types.JID{}
	for _, phone := range phones {
		participant, err := types.ParseJID(phone + "@s.whatsapp.net")
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("invalid phone %q: %v", phone, err))
			continue
		}
		participants = append(participants, participant)
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	go func() {
		ctx, cancel := netCtx()
		defer cancel()
		resp, err := client.CreateGroup(ctx, whatsmeow.ReqCreateGroup{
			Name:         subject,
			Participants: participants,
		})
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to create group: %v", err))
		} else {
			sm.uiHandler.PrintText("Group created: " + resp.JID.String())
		}
	}()
}

func cmdSubject(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "<new subject>")
		return
	}

	subject := strings.Join(params, " ")
	sm.mu.RLock()
	jidStr := sm.currentReceiver
	sm.mu.RUnlock()

	jid, err := types.ParseJID(jidStr)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid JID %q: %v", jidStr, err))
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	go func() {
		ctx, cancel := netCtx()
		defer cancel()
		if err := client.SetGroupName(ctx, jid, subject); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to set subject: %v", err))
		} else {
			sm.uiHandler.PrintText("Subject updated")
		}
	}()
}

func cmdLeave(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.mu.RLock()
	jidStr := sm.currentReceiver
	sm.mu.RUnlock()
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid JID %q: %v", jidStr, err))
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	go func() {
		ctx, cancel := netCtx()
		defer cancel()
		if err := client.LeaveGroup(ctx, jid); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to leave group: %v", err))
		} else {
			sm.uiHandler.PrintText("Left group")
		}
	}()
}

// changeParticipant implements the shared body of cmdAdd, cmdRemove,
// cmdAdmin and cmdRemoveAdmin: parse the user JID, parse the current group
// JID, check connectivity, then run the participant change off the command
// loop. verb names the action for error messages ("add", "remove",
// "promote", "demote"); done is the past-tense success prefix ("Added",
// "Removed", "Promoted", "Demoted").
func changeParticipant(sm *SessionManager, client *whatsmeow.Client, user string, change whatsmeow.ParticipantChange, verb, done string) {
	// Ensure format
	if !strings.Contains(user, "@") {
		user += "@s.whatsapp.net"
	}
	participant, err := types.ParseJID(user)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid user JID %q: %v", user, err))
		return
	}
	sm.mu.RLock()
	groupJIDStr := sm.currentReceiver
	sm.mu.RUnlock()
	groupJID, err := types.ParseJID(groupJIDStr)
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("invalid group JID %q: %v", groupJIDStr, err))
		return
	}

	if client == nil || !client.IsConnected() {
		sm.uiHandler.PrintError(errors.New("not connected"))
		return
	}

	go func() {
		ctx, cancel := netCtx()
		defer cancel()
		if _, err := client.UpdateGroupParticipants(ctx, groupJID, []types.JID{participant}, change); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to %s participant: %v", verb, err))
		} else {
			sm.uiHandler.PrintText(done + " " + user)
		}
	}()
}

func cmdAdd(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "<userid>")
		return
	}
	changeParticipant(sm, client, params[0], whatsmeow.ParticipantChangeAdd, "add", "Added")
}

func cmdRemove(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "<userid>")
		return
	}
	changeParticipant(sm, client, params[0], whatsmeow.ParticipantChangeRemove, "remove", "Removed")
}

func cmdAdmin(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "<userid>")
		return
	}
	changeParticipant(sm, client, params[0], whatsmeow.ParticipantChangePromote, "promote", "Promoted")
}

func cmdRemoveAdmin(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if !checkParam(params, 1) {
		sm.printCommandUsage(cmdName, "<userid>")
		return
	}
	changeParticipant(sm, client, params[0], whatsmeow.ParticipantChangeDemote, "demote", "Demoted")
}
