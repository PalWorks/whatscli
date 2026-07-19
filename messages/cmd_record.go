package messages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/normen/whatscli/config"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// minRecordingSize is the smallest a recorded file can be and still be
// considered real (not empty/failed) audio — a minimal valid WAV header
// alone is 44 bytes, so anything smaller means the recorder never wrote real
// data (missing binary, permission error, killed before it could open the
// output file, etc.).
const minRecordingSize = 44

// cmdRecord starts recording audio in the background via the configured
// RecordCommand, then sends the result as a voice note (PTT audio) to
// whichever chat was current when recording started — captured once here so
// switching chats mid-recording can't misdirect the eventual send. Recording
// stops (and sends) either when /stoprecording is run or automatically after
// RecordMaxSeconds.
func cmdRecord(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.mu.Lock()
	receiver := sm.currentReceiver
	alreadyRecording := sm.recording
	sm.mu.Unlock()

	if receiver == "" {
		sm.printCommandUsage(cmdName, "-> only works in a chat")
		return
	}
	if receiver == STATUSSUFFIX {
		sm.uiHandler.PrintError(errors.New("cannot send a voice note to Status (view only)"))
		return
	}
	if alreadyRecording {
		sm.uiHandler.PrintError(errors.New("already recording — use /stoprecording to stop and send"))
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

	tmpFile, err := os.CreateTemp("", "whatscli-voice-*.wav")
	if err != nil {
		sm.uiHandler.PrintError(fmt.Errorf("failed to create temp file for recording: %v", err))
		return
	}
	tmpPath := tmpFile.Name()
	// The recorder opens outPath itself; the temp file was only needed to
	// reserve a unique name.
	tmpFile.Close()

	maxDuration := time.Duration(config.Config.General.RecordMaxSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), maxDuration)

	sm.mu.Lock()
	sm.recording = true
	sm.recordCancel = cancel
	sm.mu.Unlock()

	sm.uiHandler.PrintText(fmt.Sprintf("Recording... (max %s; run /stoprecording to send sooner)", maxDuration))

	go func() {
		defer func() {
			sm.mu.Lock()
			sm.recording = false
			sm.recordCancel = nil
			sm.mu.Unlock()
			cancel()
			os.Remove(tmpPath)
		}()

		if err := runRecorder(ctx, config.Config.General.RecordCommand, tmpPath); err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("recording failed: %v", err))
			return
		}

		fi, err := os.Stat(tmpPath)
		if err != nil || fi.Size() < minRecordingSize {
			sm.uiHandler.PrintError(errors.New("recording produced no audio — check that record_command is configured correctly"))
			return
		}

		data, err := os.ReadFile(tmpPath)
		if err != nil {
			sm.uiHandler.PrintError(fmt.Errorf("failed to read recording: %v", err))
			return
		}

		// Re-fetch the client: the connection may have dropped while recording.
		liveClient := sm.getClient()
		if liveClient == nil || !liveClient.IsConnected() {
			sm.uiHandler.PrintError(errors.New("recording finished but the connection was lost — voice note not sent"))
			return
		}

		respID, err := uploadAndSendMedia(liveClient, jid, "sendaudio", data, tmpPath)
		if err != nil {
			sm.uiHandler.PrintError(err)
			return
		}
		sm.uiHandler.PrintText("Voice note sent: " + respID)
	}()
}

// cmdStopRecording ends an in-progress /record early, causing it to send
// whatever audio was captured so far instead of waiting for RecordMaxSeconds.
func cmdStopRecording(sm *SessionManager, client *whatsmeow.Client, cmdName string, params []string) {
	sm.mu.Lock()
	cancel := sm.recordCancel
	recording := sm.recording
	sm.mu.Unlock()

	if !recording || cancel == nil {
		sm.uiHandler.PrintText("not recording")
		return
	}
	cancel()
}

// runRecorder runs the configured record command, writing audio to outPath
// (appended as the command's final argument — the same convention
// ShowCommand uses for its target file; see PrintImage in ui_render.go).
//
// ctx bounds the recording: on cancellation (manual /stoprecording, or the
// RecordMaxSeconds timeout) the process is asked to exit via SIGINT rather
// than killed outright, so tools like sox/arecord get a chance to finalize a
// valid file instead of leaving a truncated one. WaitDelay bounds how long
// that grace period lasts before a hard kill.
func runRecorder(ctx context.Context, command, outPath string) error {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return errors.New("record_command is not configured")
	}
	args := append(append([]string{}, parts[1:]...), outPath)

	cmd := exec.CommandContext(ctx, parts[0], args...)
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGINT)
	}
	cmd.WaitDelay = 5 * time.Second

	err := cmd.Run()
	// A cancellation-driven stop (manual /stoprecording, or the max-duration
	// timeout) is the expected way to end a recording, not a failure — sox
	// and arecord both typically exit non-zero when interrupted. Only report
	// an error for a failure unrelated to that expected cancellation (e.g.
	// the binary doesn't exist, or exited with an error before ctx ended).
	if ctx.Err() != nil {
		return nil
	}
	return err
}
