package messages

import "go.mau.fi/whatsmeow"

func cmdHelp(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.PrintHelp()
}

func cmdQuit(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.Quit()
}

func cmdColorList(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.ShowColorList()
}

func cmdMore(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.uiHandler.PrintText("More command not implemented yet with the new backend")
}

func cmdInfo(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	if checkParam(params, 1) {
		sm.uiHandler.PrintText("Message info not yet implemented")
	} else {
		sm.printCommandUsage(cmdName, "[message-id]")
	}
}

func cmdRead(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.mu.RLock()
	receiver := sm.currentReceiver
	sm.mu.RUnlock()

	if receiver != "" {
		// TODO: Implement marking messages as read in whatsmeow
		sm.uiHandler.PrintText("Read command not implemented yet with the new backend")
	} else {
		sm.printCommandUsage(cmdName, "-> only works in a chat")
	}
}
