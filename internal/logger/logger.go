package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger writes timestamped lines to a log file in a thread-safe manner.
type Logger struct {
	mu   sync.Mutex
	file *os.File
}

// New opens (or creates) the log file at path, creating parent directories.
func New(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return &Logger{file: f}, nil
}

// Log writes a line with an ISO-8601 timestamp prefix.
func (l *Logger) Log(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(l.file, "[%s] %s\n", ts, msg)
}

// Logf formats and writes a line.
func (l *Logger) Logf(format string, args ...any) {
	l.Log(fmt.Sprintf(format, args...))
}

// Close flushes and closes the underlying file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// Discard returns a no-op Logger (useful in tests).
func Discard() *Logger {
	return &Logger{}
}

// Log on a nil/discard logger is a no-op.
func (l *Logger) log(msg string) {
	if l == nil || l.file == nil {
		return
	}
	l.Log(msg)
}
