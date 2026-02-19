package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger is the logging interface used throughout sitesync.
// Both *fileLogger and the no-op logger returned by Discard satisfy it.
type Logger interface {
	Logf(format string, args ...any)
	Close() error
}

// fileLogger writes timestamped lines to a log file in a thread-safe manner.
type fileLogger struct {
	mu   sync.Mutex
	file *os.File
}

// New opens (or creates) the log file at path, creating parent directories.
func New(path string) (Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return &fileLogger{file: f}, nil
}

// log writes a line with an ISO-8601 timestamp prefix.
func (l *fileLogger) log(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(l.file, "[%s] %s\n", ts, msg)
}

// Logf formats and writes a line.
func (l *fileLogger) Logf(format string, args ...any) {
	l.log(fmt.Sprintf(format, args...))
}

// Close flushes and closes the underlying file.
func (l *fileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// nopLogger is a no-op implementation of Logger.
type nopLogger struct{}

func (nopLogger) Logf(string, ...any) {}
func (nopLogger) Close() error        { return nil }

// Discard returns a Logger that discards all output. Safe to use concurrently.
func Discard() Logger {
	return nopLogger{}
}
