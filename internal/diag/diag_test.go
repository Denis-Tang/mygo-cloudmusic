package diag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Setup keeps the log of the day, and the errors of the session apart from
// it, while a crash is written to a file of its own.
func TestLogsErrorsAndCrashes(t *testing.T) {
	dir := t.TempDir()
	path, err := Setup(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("log: %v", err)
	}
	Errorf("测试错误 %d", 1)
	Errorf("测试错误 %d", 2)
	recent := Recent(1)
	if len(recent) != 1 || !strings.Contains(recent[0], "测试错误 2") {
		t.Fatalf("recent %v, want the last error", recent)
	}
	logged, _ := os.ReadFile(path)
	if !strings.Contains(string(logged), "ERROR 测试错误 1") {
		t.Errorf("log %q, want the errors", logged)
	}
	crash := Crash("test", "boom")
	if crash == "" {
		t.Fatal("no crash file")
	}
	body, err := os.ReadFile(crash)
	if err != nil || !strings.Contains(string(body), "boom") || !strings.Contains(string(body), "goroutine") {
		t.Fatalf("crash %q: %v", body, err)
	}
	if filepath.Dir(crash) != filepath.Join(dir, "logs") {
		t.Errorf("crash in %q", filepath.Dir(crash))
	}
}
