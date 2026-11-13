// Package diag keeps what the app logs and what crashes it: a file for each
// day under logs, the errors of the session in memory for the control server,
// and a crash file holding a panic's stack.
package diag

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"
)

const (
	keepDays  = 7
	maxErrors = 200
)

var (
	mu     sync.Mutex
	file   *os.File
	dir    string
	errs   []string
	appLog string
)

// Setup opens the log of the day under base/logs, and sends the standard
// logger there. It returns where the log is.
func Setup(base string) (string, error) {
	logDir := filepath.Join(base, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(logDir, "app-"+time.Now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	mu.Lock()
	if file != nil {
		file.Close()
	}
	file, dir, appLog = f, logDir, path
	mu.Unlock()
	log.SetOutput(io.MultiWriter(f))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	prune(logDir, keepDays)
	log.Printf("start pid=%d", os.Getpid())
	return path, nil
}

// Close closes the log of the day.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		file.Close()
		file = nil
	}
}

// Path returns the log file of the day, or "" before Setup.
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return appLog
}

// Dir returns the folder the logs and crash files are in.
func Dir() string {
	mu.Lock()
	defer mu.Unlock()
	return dir
}

// Logf writes a line to the log.
func Logf(format string, args ...any) {
	log.Printf(format, args...)
}

// Errorf writes an error to the log, and keeps it for the control server.
func Errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("ERROR %s", msg)
	mu.Lock()
	errs = append(errs, time.Now().Format("15:04:05")+" "+msg)
	if len(errs) > maxErrors {
		errs = errs[len(errs)-maxErrors:]
	}
	mu.Unlock()
}

// Recent returns the last n errors kept, the earliest first.
func Recent(n int) []string {
	mu.Lock()
	defer mu.Unlock()
	if n <= 0 || n > len(errs) {
		n = len(errs)
	}
	out := make([]string, n)
	copy(out, errs[len(errs)-n:])
	return out
}

// Crash writes what panicked, with its stack, to a crash file, and logs it.
// It returns the file's path.
func Crash(where string, r any) string {
	stack := debug.Stack()
	log.Printf("PANIC in %s: %v\n%s", where, r, stack)
	mu.Lock()
	d := dir
	mu.Unlock()
	if d == "" {
		return ""
	}
	name := filepath.Join(d, "crash-"+time.Now().Format("20060102-150405")+"-"+where+".log")
	body := fmt.Sprintf("panic in %s at %s: %v\n\n%s", where, time.Now().Format(time.RFC3339), r, stack)
	if err := os.WriteFile(name, []byte(body), 0o600); err != nil {
		return ""
	}
	mu.Lock()
	errs = append(errs, "PANIC "+where+": "+fmt.Sprint(r))
	mu.Unlock()
	return name
}

// Recover logs a panic and its stack, so a callback that panics leaves the
// app running: deferred at the top of what a goroutine or a view runs.
func Recover(where string) {
	if r := recover(); r != nil {
		Crash(where, r)
	}
}

// prune drops the logs older than days.
func prune(dir string, days int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		os.Remove(filepath.Join(dir, e.Name()))
	}
}
