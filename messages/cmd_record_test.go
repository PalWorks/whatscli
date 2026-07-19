package messages

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"testing"
	"time"

	"github.com/normen/whatscli/config"
)

// TestMain intercepts a special re-exec of this test binary acting as a fake
// audio recorder (see fakeRecorderCommand). This is the standard Go idiom
// for spawning a real child process with genuine OS signal handling in a
// test (used by the standard library's own os/exec tests) — critically more
// faithful than a shell script here: an earlier version of these tests used
// a `trap ... INT; sleep 10` shell script to stand in for sox/arecord, but
// dash (this system's /bin/sh) does not run a trap handler while blocked in
// a non-interactive wait() on a foreground child like `sleep`, so the
// simulated "recorder" never responded to SIGINT — a flaw in the test
// double, not in runRecorder. A real recorder binary (sox, arecord) is a
// single process with its own signal handler blocked in a read() on an audio
// device, which responds to SIGINT immediately; this helper process
// reproduces that shape correctly via signal.Notify.
func TestMain(m *testing.M) {
	if os.Getenv("WHATSCLI_TEST_FAKE_RECORDER") == "1" {
		os.Exit(fakeRecorderMain())
	}
	os.Exit(m.Run())
}

// fakeRecorderMain is the body of the re-exec'd fake recorder process. It
// writes recordingByteCount zero bytes to its last argument (the output path
// runRecorder appends) either immediately (an instant, non-interactive
// "recording") or upon receiving SIGINT (simulating a real recorder that
// finalizes its output file when interrupted), whichever the test needs;
// which behavior applies is controlled by WHATSCLI_TEST_FAKE_RECORDER_WAIT.
func fakeRecorderMain() int {
	args := os.Args
	outPath := args[len(args)-1]

	if os.Getenv("WHATSCLI_TEST_FAKE_RECORDER_WAIT") == "1" {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		select {
		case <-sigCh:
		case <-time.After(10 * time.Second):
			return 1 // never interrupted — the test that expected a stop failed
		}
	}

	if err := os.WriteFile(outPath, make([]byte, fakeRecordingByteCount), 0o644); err != nil {
		return 1
	}
	return 0
}

const fakeRecordingByteCount = 100

// fakeRecorderCommand returns a RecordCommand value that re-execs this test
// binary in fake-recorder mode (see TestMain). wait selects whether it
// writes its output immediately (a normal completed recording) or only after
// receiving SIGINT (simulating a recording stopped early via /stoprecording
// or the RecordMaxSeconds timeout).
func fakeRecorderCommand(t *testing.T, wait bool) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable failed: %v", err)
	}
	t.Setenv("WHATSCLI_TEST_FAKE_RECORDER", "1")
	if wait {
		t.Setenv("WHATSCLI_TEST_FAKE_RECORDER_WAIT", "1")
	}
	return exe
}

// ----- Phase 5.7: cmdRecord guards (synchronous, no subprocess involved) -----

func TestCmdRecord_NoReceiver(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdRecord(sm, nil, "record", nil)

	if len(mock.Texts) != 1 {
		t.Fatalf("expected exactly 1 usage message, got %d: %v", len(mock.Texts), mock.Texts)
	}
}

func TestCmdRecord_RefusesStatusBroadcast(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = STATUSSUFFIX
	sm.mu.Unlock()

	cmdRecord(sm, nil, "record", nil)

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "cannot send a voice note to Status (view only)" {
		t.Fatalf("expected the status-is-read-only error, got %v", mock.Errors)
	}
}

func TestCmdRecord_NilClient(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "peer@s.whatsapp.net"
	sm.mu.Unlock()

	cmdRecord(sm, nil, "record", nil)

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

func TestCmdRecord_RejectsConcurrentRecording(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "peer@s.whatsapp.net"
	sm.recording = true
	sm.mu.Unlock()

	cmdRecord(sm, nil, "record", nil)

	if len(mock.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(mock.Errors), mock.Errors)
	}
}

func TestCmdStopRecording_NotRecording(t *testing.T) {
	sm, mock := newTestSessionManager(t)

	cmdStopRecording(sm, nil, "stoprecording", nil)

	if len(mock.Texts) != 1 || mock.Texts[0] != "not recording" {
		t.Fatalf("expected 'not recording', got %v", mock.Texts)
	}
}

// ----- Phase 5.7: runRecorder (real subprocess) -----

func TestRunRecorder_EmptyCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runRecorder(ctx, "", filepath.Join(t.TempDir(), "out.wav")); err == nil {
		t.Error("expected an error for an empty record_command")
	}
}

func TestRunRecorder_MissingBinary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := runRecorder(ctx, "/nonexistent-binary-xyz-whatscli-test", filepath.Join(t.TempDir(), "out.wav"))
	if err == nil {
		t.Error("expected an error for a nonexistent recorder binary")
	}
}

func TestRunRecorder_WritesOutputFile(t *testing.T) {
	command := fakeRecorderCommand(t, false)
	outPath := filepath.Join(t.TempDir(), "out.wav")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runRecorder(ctx, command, outPath); err != nil {
		t.Fatalf("runRecorder error: %v", err)
	}

	fi, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}
	if fi.Size() != fakeRecordingByteCount {
		t.Errorf("expected %d bytes written, got %d", fakeRecordingByteCount, fi.Size())
	}
}

func TestRunRecorder_CancellationIsNotAnError(t *testing.T) {
	// The fake recorder blocks until SIGINT, then writes its output and
	// exits — runRecorder must treat the resulting cancellation-driven exit
	// as success (ctx.Err() will be non-nil), not propagate it as an error.
	command := fakeRecorderCommand(t, true)
	outPath := filepath.Join(t.TempDir(), "out.wav")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if err := runRecorder(ctx, command, outPath); err != nil {
		t.Fatalf("expected cancellation-driven exit to be treated as success, got: %v", err)
	}

	fi, err := os.Stat(outPath)
	if err != nil || fi.Size() != fakeRecordingByteCount {
		t.Fatalf("expected the interrupt handler to have written %d bytes, err=%v", fakeRecordingByteCount, err)
	}
}

// ----- Phase 5.7: cmdRecord/cmdStopRecording guard-chain integration -----

func TestCmdRecord_HappyPath_ReachesConnectivityGuard(t *testing.T) {
	orig := config.Config.General.RecordCommand
	origMax := config.Config.General.RecordMaxSeconds
	config.Config.General.RecordCommand = fakeRecorderCommand(t, false)
	config.Config.General.RecordMaxSeconds = 5
	defer func() {
		config.Config.General.RecordCommand = orig
		config.Config.General.RecordMaxSeconds = origMax
	}()

	sm, mock := newTestSessionManager(t)
	sm.mu.Lock()
	sm.currentReceiver = "peer@s.whatsapp.net"
	sm.mu.Unlock()

	// cmdRecord's connectivity guard runs before the recording goroutine is
	// ever spawned, so a nil client (the only kind constructible in this
	// test environment, which has no live WhatsApp connection) stops here.
	// The full success path — spawning the recorder, uploading, and
	// sending — is exercised by TestRunRecorder_* above for the subprocess
	// half, and by the existing cmdMedia tests for the upload/send half
	// (uploadAndSendMedia is shared code); only the live-network glue
	// between them can't be exercised without a real WhatsApp account.
	cmdRecord(sm, nil, "record", nil)

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "not connected" {
		t.Fatalf("expected 'not connected', got %v", mock.Errors)
	}
}

func TestCmdStopRecording_CancelsInProgressRecording(t *testing.T) {
	command := fakeRecorderCommand(t, true)

	sm, _ := newTestSessionManager(t)

	// Drive the recording lifecycle directly (bypassing cmdRecord's
	// nil-client connectivity guard, already covered above) to exercise the
	// real subprocess start/signal/stop mechanics end to end.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	sm.mu.Lock()
	sm.recording = true
	sm.recordCancel = cancel
	sm.mu.Unlock()

	done := make(chan error, 1)
	outPath := filepath.Join(t.TempDir(), "stoptest.wav")
	go func() {
		done <- runRecorder(ctx, command, outPath)
	}()

	// Give the fake recorder process a moment to actually start and install
	// its signal.Notify handler before signaling. Unlike the shell-script
	// approach this replaced, a real process's signal handler is armed
	// within microseconds of starting, so this fixed, generous margin is not
	// a meaningful source of flakiness.
	time.Sleep(300 * time.Millisecond)

	cmdStopRecording(sm, nil, "stoprecording", nil)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected the SIGINT-driven stop to be treated as success, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runRecorder did not return after /stoprecording")
	}

	fi, err := os.Stat(outPath)
	if err != nil || fi.Size() != fakeRecordingByteCount {
		t.Fatalf("expected the fake recorder to have written its output before exiting, err=%v", err)
	}
}
