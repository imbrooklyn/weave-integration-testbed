package demoapp

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/imbrooklyn/weave-integration-testbed/internal/scenario"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
)

func TestRunMemoryWritesReportAndReturnsSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunMemory(nil, &stdout, &stderr, func(context.Context) (scenario.Report, error) {
		return scenario.Report{
			Adapter: "memory",
			Backend: "reference",
			Results: []scenario.Result{{Name: "one", IDs: []string{"r01"}}},
		}, nil
	})
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "1 passed") {
		t.Fatalf("RunMemory() = code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestRunMemoryReturnsExecutionFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunMemory(nil, &stdout, &stderr, func(context.Context) (scenario.Report, error) {
		return scenario.Report{}, errors.New("controlled failure")
	})
	if code != 1 || !strings.Contains(stderr.String(), "controlled failure") {
		t.Fatalf("RunMemory() = code %d, stderr %q", code, stderr.String())
	}
}

func TestRunSQLUsesStableBackendOrder(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var called []testenv.Backend
	code := RunSQL("gorm", nil, &stdout, &stderr, func(
		_ context.Context,
		backend testenv.Backend,
	) (scenario.Report, error) {
		called = append(called, backend)
		return scenario.Report{Adapter: "gorm", Backend: string(backend)}, nil
	})
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("RunSQL() = code %d, stderr %q", code, stderr.String())
	}
	if want := []testenv.Backend{testenv.MySQL, testenv.PostgreSQL}; !reflect.DeepEqual(called, want) {
		t.Fatalf("RunSQL() backends = %v, want %v", called, want)
	}
}

func TestRunDocumentWritesReportAndReturnsSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunDocument(
		"mongo",
		nil,
		&stdout,
		&stderr,
		func(context.Context) (scenario.Report, error) {
			return scenario.Report{
				Adapter: "mongo",
				Backend: "mongodb-6.0.28",
				Results: []scenario.Result{{Name: "one", IDs: []string{"r01"}}},
			}, nil
		},
	)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "1 passed") {
		t.Fatalf(
			"RunDocument() = code %d, stdout %q, stderr %q",
			code,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunDocumentRejectsInvalidUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunDocument("mongo", []string{"--timeout=0"}, &stdout, &stderr, nil)
	if code != 2 || !strings.Contains(stderr.String(), "timeout must be positive") {
		t.Fatalf("RunDocument() = code %d, stderr %q", code, stderr.String())
	}
}

func TestRunSQLRejectsInvalidUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunSQL("gorm", []string{"--timeout=0"}, &stdout, &stderr, nil)
	if code != 2 || !strings.Contains(stderr.String(), "timeout must be positive") {
		t.Fatalf("RunSQL() = code %d, stderr %q", code, stderr.String())
	}
}
