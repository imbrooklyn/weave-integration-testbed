//go:build integration

package integration

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/imbrooklyn/weave-integration-testbed/internal/demoharness"
	"github.com/imbrooklyn/weave-integration-testbed/internal/scenario"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
)

const backendTimeout = 2 * time.Minute

func TestCanonicalMatchSetMatrix(t *testing.T) {
	referenceContext, cancelReference := context.WithTimeout(
		context.Background(),
		backendTimeout,
	)
	reference, err := demoharness.RunMemory(referenceContext)
	cancelReference()
	if err != nil {
		t.Fatalf("run memory reference: %v", err)
	}
	assertReportShape(t, reference, 28, 0)
	logReport(t, reference)

	for _, backend := range testenv.SQLBackends() {
		t.Run(string(backend), func(t *testing.T) {
			resetFixture(t, backend)

			gormgenReport := runWithTimeout(t, backend, demoharness.RunGORMGen)
			assertReportShape(t, gormgenReport, 27, 1)
			if err := scenario.CompareReference(reference, gormgenReport); err != nil {
				t.Fatalf("compare GORM Gen with memory reference: %v", err)
			}
			logReport(t, gormgenReport)

			gormReport := runWithTimeout(t, backend, demoharness.RunGORM)
			assertReportShape(t, gormReport, 27, 1)
			if err := scenario.CompareReference(reference, gormReport); err != nil {
				t.Fatalf("compare GORM with memory reference: %v", err)
			}
			if err := scenario.CompareEquivalent(gormgenReport, gormReport); err != nil {
				t.Fatalf("compare GORM Gen with GORM: %v", err)
			}
			logReport(t, gormReport)
		})
	}
}

func runWithTimeout(
	t *testing.T,
	backend testenv.Backend,
	run func(context.Context, testenv.Backend) (scenario.Report, error),
) scenario.Report {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backendTimeout)
	defer cancel()
	report, err := run(ctx, backend)
	if err != nil {
		t.Fatalf("run %s backend: %v", backend, err)
	}
	return report
}

func resetFixture(t *testing.T, backend testenv.Backend) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backendTimeout)
	defer cancel()
	config, err := testenv.LoadSQLConfig(backend)
	if err != nil {
		t.Fatalf("load %s configuration: %v", backend, err)
	}
	database, err := testenv.OpenSQL(config)
	if err != nil {
		t.Fatalf("open %s fixture database: %v", backend, err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close %s fixture database: %v", backend, err)
		}
	})
	if err := testenv.WaitForSQL(ctx, backend, database, 250*time.Millisecond); err != nil {
		t.Fatalf("wait for %s fixture database: %v", backend, err)
	}
	if err := testenv.ResetSQL(ctx, database, repositoryRoot(t), backend); err != nil {
		t.Fatalf("reset %s fixture: %v", backend, err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test source")
	}
	return filepath.Dir(filepath.Dir(filename))
}

func assertReportShape(
	t *testing.T,
	report scenario.Report,
	wantResults int,
	wantSkipped int,
) {
	t.Helper()
	if got := len(report.Results); got != wantResults {
		t.Fatalf(
			"%s/%s result count = %d, want %d",
			report.Adapter,
			report.Backend,
			got,
			wantResults,
		)
	}
	if got := len(report.Skipped); got != wantSkipped {
		t.Fatalf(
			"%s/%s skipped count = %d, want %d",
			report.Adapter,
			report.Backend,
			got,
			wantSkipped,
		)
	}
	if wantSkipped == 1 && report.Skipped[0] != "missing state" {
		t.Fatalf(
			"%s/%s skipped scenarios = %v, want [missing state]",
			report.Adapter,
			report.Backend,
			report.Skipped,
		)
	}
}

func logReport(t *testing.T, report scenario.Report) {
	t.Helper()
	for _, result := range report.Results {
		t.Logf(
			"match-set %s/%s %q => %v",
			report.Adapter,
			report.Backend,
			result.Name,
			result.IDs,
		)
	}
	for _, name := range report.Skipped {
		t.Logf(
			"match-set %s/%s %q => skipped: missing is materialized as null",
			report.Adapter,
			report.Backend,
			name,
		)
	}
}
