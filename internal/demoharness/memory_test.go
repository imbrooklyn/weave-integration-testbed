package demoharness

import (
	"context"
	"testing"
	"time"
)

func TestMemoryReferenceMatchSets(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := RunMemory(ctx)
	if err != nil {
		t.Fatalf("RunMemory() error = %v", err)
	}
	if got, want := len(report.Results), 28; got != want {
		t.Fatalf("memory result count = %d, want %d", got, want)
	}
	if len(report.Skipped) != 0 {
		t.Fatalf("memory skipped scenarios = %v, want none", report.Skipped)
	}
}
