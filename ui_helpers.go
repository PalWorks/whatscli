package main

import (
	"fmt"

	"github.com/normen/whatscli/config"
	"github.com/normen/whatscli/messages"
)

// prints text to the TextView
func PrintText(txt string) {
	fmt.Fprintln(ctx.TextView, txt)
}

// prints an error to the TextView
func PrintError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(ctx.TextView, "["+config.Config.Colors.Negative+"]", err.Error(), "[-]")
}

// prints an error to the TextView
func PrintErrorMsg(text string, err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(ctx.TextView, "["+config.Config.Colors.Negative+"]", text, err.Error(), "[-]")
}

// prints help to chat view
func PrintHelp() {
	cmdPrefix := config.Config.General.CmdPrefix
	fmt.Fprintln(ctx.TextView, "")
	fmt.Fprintln(ctx.TextView, "[::b]Application & Connection[::-]")
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"connect / "+config.Config.Keymap.CommandConnect, "(Re)Connect"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"disconnect", "Disconnect"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"logout", "Logout & Delete Data"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"quit / "+config.Config.Keymap.CommandQuit, "Exit"))
	fmt.Fprintln(ctx.TextView, "")

	fmt.Fprintln(ctx.TextView, "[::b]Navigation & View[::-]")
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", "Up/Down", "Scroll Chats/History"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.SwitchPanels, "Switch Panel"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.FocusMessages, "Focus Messages"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", "Ctrl+p", "Toggle Mouse"))
	fmt.Fprintln(ctx.TextView, "")

	fmt.Fprintln(ctx.TextView, "[::b]Chat & Messages[::-]")
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"backlog / "+config.Config.Keymap.CommandBacklog, "Load Backlog"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"read / "+config.Config.Keymap.CommandRead, "Mark Read"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", "Up/Down", "Select Message"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.MessageDownload, "Download"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.MessageOpen, "Download & Open"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.MessageShow, "Show Image ("+config.Config.General.ShowCommand+")"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.MessageUrl, "Open URL"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.MessageRevoke, "Revoke"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.MessageInfo, "Info"))
	fmt.Fprintln(ctx.TextView, "")

	fmt.Fprintln(ctx.TextView, "[::b]Media[::-]")
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"upload <path>", "Send File"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"sendimage <path>", "Send Image"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"sendvideo <path>", "Send Video"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"sendaudio <path>", "Send Audio"))
	fmt.Fprintln(ctx.TextView, "")

	fmt.Fprintln(ctx.TextView, "[::b]Search[::-]")
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"search <keyword>", "Search Messages"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"search-contact <name>", "Find Contacts"))
	fmt.Fprintln(ctx.TextView, "")

	fmt.Fprintln(ctx.TextView, "[::b]Groups[::-]")
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"create <nums> <subj>", "Create"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"subject <subj>", "Subject"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"leave", "Leave"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"add <userid>", "Add"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"remove <userid>", "Kick"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"admin <userid>", "Promote"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", cmdPrefix+"removeadmin <userid>", "Demote"))
	fmt.Fprintln(ctx.TextView, "")

	fmt.Fprintln(ctx.TextView, "[::b]Clipboard[::-]")
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.Copyuser, "Copy UserID"))
	fmt.Fprintln(ctx.TextView, fmt.Sprintf("[::b] %-30s [::-]= %s", config.Config.Keymap.Pasteuser, "Paste UserID"))
	fmt.Fprintln(ctx.TextView, "")

	fmt.Fprintln(ctx.TextView, "Config: ", config.GetConfigFilePath())
	fmt.Fprintln(ctx.TextView, "Verify: ", VERSION)
}

// updates the status bar
func UpdateStatusBar(statusInfo messages.SessionStatus) {
	out := " "
	if statusInfo.Connected {
		out += "[" + config.Config.Colors.Positive + "]online[-]"
	} else {
		out += "[" + config.Config.Colors.Negative + "]offline[-]"
	}
	out += " "
	out += "[::d] ("
	out += fmt.Sprint(statusInfo.BatteryCharge)
	out += "%"
	if statusInfo.BatteryLoading {
		out += " [" + config.Config.Colors.Positive + "]L[-]"
	} else {
		out += " [" + config.Config.Colors.Negative + "]l[-]"
	}
	if statusInfo.BatteryPowersave {
		out += " [" + config.Config.Colors.Negative + "]S[-]"
	} else {
		out += " [" + config.Config.Colors.Positive + "]s[-]"
	}
	out += ")[::-] "
	out += statusInfo.LastSeen
}

// sets the current chat, loads text from storage to TextView
func SetDisplayedChat(chat messages.Chat) {
	ctx.CurrentReceiver = chat
	ctx.TextView.Clear()
	ctx.TextView.SetTitle(chat.Name)
	ctx.SessionManager.CommandChannel <- messages.Command{Name: "select", Params: []string{ctx.CurrentReceiver.Id}}
}
