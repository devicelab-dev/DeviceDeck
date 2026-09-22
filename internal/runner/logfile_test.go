package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

func TestSetLogFileCapturesDriverDiagnostics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner.log")
	if err := SetLogFile(path); err != nil {
		t.Fatal(err)
	}
	logger.Info("driver installed on %s", "emulator-5554")
	CloseLogFile()
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "driver installed on emulator-5554") {
		t.Fatalf("runner log = %q, %v", b, err)
	}
}

func TestSetLogFileError(t *testing.T) {
	if err := SetLogFile(filepath.Join(t.TempDir(), "missing", "runner.log")); err == nil {
		t.Fatal("expected an error for a missing folder")
	}
}
