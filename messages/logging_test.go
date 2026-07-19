package messages

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Note: defaultDebugLogPath (and by extension SessionManager.Init()'s
// debug-log wiring) is intentionally not tested here. Both ultimately call
// config.GetSessionFilePath(), which resolves through the same adrg/xdg
// package discussed in cmd_connection_test.go — and per that package's own
// doc comments, XDG*File functions create their parent directory as a side
// effect if it doesn't exist, even though they don't create the file
// itself. Calling it in a test would create the real ~/.config/whatscli/
// directory. openDebugLog and everything below is tested with an explicit
// path instead, which never reaches that code path.

func TestFileLogger_WritesLeveledLines(t *testing.T) {
	var buf bytes.Buffer
	logger := newFileLogger("TEST", &buf)

	logger.Infof("info message %d", 1)
	logger.Warnf("warn message")
	logger.Errorf("error message: %v", errors.New("boom"))
	logger.Debugf("debug message")

	out := buf.String()
	for _, want := range []string{"[TEST]", "INFO", "info message 1", "WARN", "warn message", "ERROR", "error message: boom", "DEBUG", "debug message"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected log output to contain %q, got:\n%s", want, out)
		}
	}

	// Four calls should produce four lines.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 log lines, got %d: %v", len(lines), lines)
	}
}

func TestFileLogger_SubAppendsModulePath(t *testing.T) {
	var buf bytes.Buffer
	root := newFileLogger("whatscli", &buf)
	sub := root.Sub("CLIENT")

	sub.Infof("hello")

	if !strings.Contains(buf.String(), "[whatscli/CLIENT]") {
		t.Errorf("expected sub-logger module path 'whatscli/CLIENT', got: %s", buf.String())
	}
}

func TestFileLogger_ConcurrentWritesDoNotInterleave(t *testing.T) {
	// fileLogger's own mutex (in write()) serializes every call before it
	// ever reaches the underlying io.Writer, so a plain bytes.Buffer here
	// is sufficient — this test is verifying that serialization, not
	// working around a lack of it.
	var buf bytes.Buffer
	logger := newFileLogger("TEST", &buf)

	const n = 50
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			logger.Infof("line %d", i)
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < n; i++ {
		<-done
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != n {
		t.Errorf("expected %d complete, non-interleaved lines, got %d", n, len(lines))
	}
}

func TestLoggingUiHandler_PrintErrorLogsAndDelegates(t *testing.T) {
	var buf bytes.Buffer
	mock := NewMockUiHandler()
	handler := &loggingUiHandler{UiMessageHandler: mock, logger: newFileLogger("whatscli", &buf)}

	handler.PrintError(errors.New("connection lost"))

	if len(mock.Errors) != 1 || mock.Errors[0].Error() != "connection lost" {
		t.Errorf("expected the wrapped handler to still receive the error, got %v", mock.Errors)
	}
	if !strings.Contains(buf.String(), "connection lost") {
		t.Errorf("expected the error to also be written to the log, got: %s", buf.String())
	}
}

func TestLoggingUiHandler_PrintErrorNil_NoLogLine(t *testing.T) {
	var buf bytes.Buffer
	mock := NewMockUiHandler()
	handler := &loggingUiHandler{UiMessageHandler: mock, logger: newFileLogger("whatscli", &buf)}

	handler.PrintError(nil)

	if buf.Len() != 0 {
		t.Errorf("expected no log line for a nil error, got: %s", buf.String())
	}
	if len(mock.Errors) != 1 || mock.Errors[0] != nil {
		t.Errorf("expected the wrapped handler to still receive the nil error, got %v", mock.Errors)
	}
}

func TestLoggingUiHandler_PrintTextLogsAndDelegates(t *testing.T) {
	var buf bytes.Buffer
	mock := NewMockUiHandler()
	handler := &loggingUiHandler{UiMessageHandler: mock, logger: newFileLogger("whatscli", &buf)}

	handler.PrintText("connected")

	if len(mock.Texts) != 1 || mock.Texts[0] != "connected" {
		t.Errorf("expected the wrapped handler to still receive the text, got %v", mock.Texts)
	}
	if !strings.Contains(buf.String(), "connected") {
		t.Errorf("expected the text to also be written to the log, got: %s", buf.String())
	}
}

func TestLoggingUiHandler_DelegatesOtherMethodsUnchanged(t *testing.T) {
	mock := NewMockUiHandler()
	handler := &loggingUiHandler{UiMessageHandler: mock, logger: newFileLogger("whatscli", &bytes.Buffer{})}

	// Spot-check a few of the many delegated (non-overridden) methods.
	handler.PrintHelp()
	handler.Quit()
	handler.ShowColorList()

	if mock.HelpCalled != 1 {
		t.Errorf("expected PrintHelp to be delegated, got HelpCalled=%d", mock.HelpCalled)
	}
	if mock.QuitCalled != 1 {
		t.Errorf("expected Quit to be delegated, got QuitCalled=%d", mock.QuitCalled)
	}
	if mock.ColorCalled != 1 {
		t.Errorf("expected ShowColorList to be delegated, got ColorCalled=%d", mock.ColorCalled)
	}
}

func TestOpenDebugLog_ExplicitPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "debug.log")

	f, err := openDebugLog(path)
	if err != nil {
		t.Fatalf("openDebugLog error: %v", err)
	}
	defer f.Close()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected log file to exist at %s: %v", path, err)
	}
}

func TestOpenDebugLog_TruncatesOnEachCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")

	f1, err := openDebugLog(path)
	if err != nil {
		t.Fatalf("first openDebugLog error: %v", err)
	}
	if _, err := f1.WriteString("old run's log content"); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	f1.Close()

	f2, err := openDebugLog(path)
	if err != nil {
		t.Fatalf("second openDebugLog error: %v", err)
	}
	defer f2.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected the log file to be truncated on re-open, got %d bytes: %q", len(data), data)
	}
}
