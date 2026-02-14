package messages

import (
	"context"
	"fmt"
	"os"

	"go.mau.fi/whatsmeow"

	"github.com/normen/whatscli/config"
)

func cmdLogin(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	err := sm.login()
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("WhatsApp connection failed: %v", err))
		sm.uiHandler.PrintText("Try using /reset to completely reset the connection")
	} else {
		sm.uiHandler.PrintText("Successfully connected to WhatsApp")
	}
}

func cmdDisconnect(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.PrintError(sm.disconnect())
}

func cmdLogout(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.PrintError(sm.logout())
}

func cmdReset(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	// Fully reset everything — must use sm.client directly under write lock
	// (not the captured client parameter) because we need to nil it out.
	sm.mu.Lock()
	localClient := sm.client
	if localClient != nil {
		if localClient.IsConnected() {
			localClient.Disconnect()
		}

		if localClient.Store != nil {
			err := localClient.Store.Delete(context.Background())
			if err != nil {
				sm.uiHandler.PrintText("Warning: Couldn't remove session: " + err.Error())
			}
		}
	}

	sm.client = nil
	sm.container = nil
	sm.mu.Unlock()

	// Remove the DB file
	dbPath := config.GetSessionFilePath() + ".db"
	err := os.Remove(dbPath)
	if err != nil && !os.IsNotExist(err) {
		sm.uiHandler.PrintText("Warning: Couldn't remove database file: " + err.Error())
	}

	sm.uiHandler.PrintText("Session reset. Use /connect to reconnect with a new QR code.")
	sm.StatusChannel <- StatusMsg{false, nil}
}
