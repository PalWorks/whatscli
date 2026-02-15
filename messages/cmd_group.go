package messages

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func cmdCreate(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if checkParam(params, 2) {
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

		if client != nil {
			resp, err := client.CreateGroup(context.Background(), whatsmeow.ReqCreateGroup{
				Name:         subject,
				Participants: participants,
			})
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("failed to create group: %v", err))
			} else {
				sm.uiHandler.PrintText("Group created: " + resp.JID.String())
			}
		}
	} else {
		sm.printCommandUsage(cmdName, "<phone1,phone2> <subject>")
	}
}

func cmdSubject(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if checkParam(params, 1) {
		subject := strings.Join(params, " ")
		sm.mu.RLock()
		jidStr := sm.currentReceiver
		sm.mu.RUnlock()

		jid, err := types.ParseJID(jidStr)
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("invalid JID %q: %v", jidStr, err))
			return
		}
		if client != nil {
			err := client.SetGroupName(context.Background(), jid, subject)
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("failed to set subject: %v", err))
			} else {
				sm.uiHandler.PrintText("Subject updated")
			}
		}
	} else {
		sm.printCommandUsage(cmdName, "<new subject>")
	}
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
	if client != nil {
		err := client.LeaveGroup(context.Background(), jid)
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to leave group: %v", err))
		} else {
			sm.uiHandler.PrintText("Left group")
		}
	}
}

func cmdAdd(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if checkParam(params, 1) {
		user := params[0]
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

		if client != nil {
			_, err := client.UpdateGroupParticipants(context.Background(), groupJID, []types.JID{participant}, whatsmeow.ParticipantChangeAdd)
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("failed to add participant: %v", err))
			} else {
				sm.uiHandler.PrintText("Added " + user)
			}
		}
	} else {
		sm.printCommandUsage(cmdName, "<userid>")
	}
}

func cmdRemove(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if checkParam(params, 1) {
		user := params[0]
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

		if client != nil {
			_, err := client.UpdateGroupParticipants(context.Background(), groupJID, []types.JID{participant}, whatsmeow.ParticipantChangeRemove)
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("failed to remove participant: %v", err))
			} else {
				sm.uiHandler.PrintText("Removed " + user)
			}
		}
	} else {
		sm.printCommandUsage(cmdName, "<userid>")
	}
}

func cmdAdmin(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if checkParam(params, 1) {
		user := params[0]
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

		if client != nil {
			_, err := client.UpdateGroupParticipants(context.Background(), groupJID, []types.JID{participant}, whatsmeow.ParticipantChangePromote)
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("failed to promote participant: %v", err))
			} else {
				sm.uiHandler.PrintText("Promoted " + user)
			}
		}
	} else {
		sm.printCommandUsage(cmdName, "<userid>")
	}
}

func cmdRemoveAdmin(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if checkParam(params, 1) {
		user := params[0]
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

		if client != nil {
			_, err := client.UpdateGroupParticipants(context.Background(), groupJID, []types.JID{participant}, whatsmeow.ParticipantChangeDemote)
			if err != nil {
				sm.uiHandler.PrintError(fmt.Errorf("failed to demote participant: %v", err))
			} else {
				sm.uiHandler.PrintText("Demoted " + user)
			}
		}
	} else {
		sm.printCommandUsage(cmdName, "<userid>")
	}
}
