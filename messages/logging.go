package messages

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/normen/whatscli/config"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// fileLogger is a minimal waLog.Logger implementation that writes
// timestamped, leveled log lines to an io.Writer (typically an open debug
// log file). All loggers created via Sub() share the same underlying mutex
// so concurrent writers (whatsmeow uses many goroutines) never interleave
// partial lines.
type fileLogger struct {
	module string
	out    io.Writer
	mu     *sync.Mutex
}

// newFileLogger creates a root fileLogger writing to out.
func newFileLogger(module string, out io.Writer) waLog.Logger {
	return &fileLogger{module: module, out: out, mu: &sync.Mutex{}}
}

func (l *fileLogger) write(level, msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.out, "%s [%s] %-5s %s\n", time.Now().Format("2006-01-02 15:04:05.000"), l.module, level, fmt.Sprintf(msg, args...))
}

func (l *fileLogger) Warnf(msg string, args ...any)  { l.write("WARN", msg, args...) }
func (l *fileLogger) Errorf(msg string, args ...any) { l.write("ERROR", msg, args...) }
func (l *fileLogger) Infof(msg string, args ...any)  { l.write("INFO", msg, args...) }
func (l *fileLogger) Debugf(msg string, args ...any) { l.write("DEBUG", msg, args...) }

func (l *fileLogger) Sub(module string) waLog.Logger {
	return &fileLogger{module: l.module + "/" + module, out: l.out, mu: l.mu}
}

// loggingUiHandler wraps a UiMessageHandler so that every PrintError and
// PrintText call is also written to the debug logger, in addition to going
// to the UI as normal. This gives a post-mortem log file for connection
// failures and status messages even after the TUI has closed — everything
// else is delegated unchanged via the embedded interface.
type loggingUiHandler struct {
	UiMessageHandler
	logger waLog.Logger
}

func (h *loggingUiHandler) PrintError(err error) {
	if err != nil {
		h.logger.Errorf("%v", err)
	}
	h.UiMessageHandler.PrintError(err)
}

func (h *loggingUiHandler) PrintText(text string) {
	h.logger.Infof("%s", text)
	h.UiMessageHandler.PrintText(text)
}

// defaultDebugLogPath returns <config dir>/whatscli/debug.log, alongside
// the existing session.db / session_meta.db files.
func defaultDebugLogPath() string {
	return filepath.Join(filepath.Dir(config.GetSessionFilePath()), "debug.log")
}

// openDebugLog truncates (crude rotation — one run, one log) and opens the
// debug log file at path, or the default path if path is empty.
func openDebugLog(path string) (*os.File, error) {
	if path == "" {
		path = defaultDebugLogPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}
	f, err := os.Create(path) // O_TRUNC|O_CREATE|O_WRONLY
	if err != nil {
		return nil, fmt.Errorf("failed to open debug log %q: %w", path, err)
	}
	return f, nil
}
