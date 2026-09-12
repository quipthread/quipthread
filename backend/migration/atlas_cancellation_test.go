package migration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAtlasRunnerCleansTempFilesAndReturnsUnknownAfterCancellation(t *testing.T) {
	recordPath := filepath.Join(t.TempDir(), "record")
	binaryPath := fakeAtlasBinary(t, recordPath, "cancel")
	runner, err := NewRunner(Config{BinaryPath: binaryPath, Version: AtlasCLIVersion, Checksum: fileSHA256(t, binaryPath), TempRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	var result Outcome
	var applyErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		result, applyErr = runner.Apply(ctx, Request{AccountID: "account-cancel", TargetURL: "libsql://cancel.example.test", AuthToken: "secret"})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("migration child did not stop during cleanup")
		}
	})
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

waitingForApply:
	for {
		select {
		case <-done:
			t.Fatalf("migration returned before cancellation: outcome=%#v err=%v", result, applyErr)
		case <-ticker.C:
			data, err := os.ReadFile(recordPath)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "args=migrate apply ") {
				break waitingForApply
			}
		}
	}
	cancel()
	watchdog := time.NewTimer(5 * time.Second)
	defer watchdog.Stop()
	select {
	case <-done:
	case <-watchdog.C:
		t.Fatal("migration apply did not return after cancellation")
	}
	if result.Kind != OutcomeMigrationUnknown || !errors.As(applyErr, new(*OutcomeError)) || !errors.Is(applyErr, context.Canceled) {
		t.Fatalf("cancelled migration outcome=%#v err=%v", result, applyErr)
	}
	entries, err := os.ReadDir(runner.tempRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("runner temp root entries after cancellation = %v", entries)
	}
}
