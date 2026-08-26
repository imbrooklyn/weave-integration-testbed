package testenv

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imbrooklyn/weave-integration-testbed/internal/fixture"
)

const scriptBoundary = "-- weave:split"

// WaitForSQL waits until an authenticated PingContext succeeds or the caller's
// deadline expires. Driver errors are intentionally omitted from the returned
// message so connection details cannot leak through logs.
func WaitForSQL(
	ctx context.Context,
	backend Backend,
	database *sql.DB,
	interval time.Duration,
) error {
	if ctx == nil {
		return fmt.Errorf("wait for %s: nil context", backend)
	}
	if database == nil {
		return fmt.Errorf("wait for %s: nil database handle", backend)
	}
	if interval <= 0 {
		return fmt.Errorf("wait for %s: interval must be positive", backend)
	}

	for {
		if err := database.PingContext(ctx); err == nil {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf(
				"%s database did not become healthy: %w",
				backend,
				ctx.Err(),
			)
		case <-timer.C:
		}
	}
}

// ResetSQL replays the backend DDL and seed files from the repository root.
func ResetSQL(
	ctx context.Context,
	database *sql.DB,
	repositoryRoot string,
	backend Backend,
) error {
	if ctx == nil {
		return fmt.Errorf("reset %s fixture: nil context", backend)
	}
	if database == nil {
		return fmt.Errorf("reset %s fixture: nil database handle", backend)
	}
	if _, ok := defaultsByBackend[backend]; !ok {
		return fmt.Errorf("reset fixture: unsupported SQL backend %q", backend)
	}
	if strings.TrimSpace(repositoryRoot) == "" {
		return fmt.Errorf("reset %s fixture: empty repository root", backend)
	}

	base := filepath.Join(repositoryRoot, "testdata", string(backend))
	paths := []string{
		filepath.Join(base, "001_schema.sql"),
		filepath.Join(base, "002_seed.sql"),
	}
	for _, path := range paths {
		statements, err := readSQLStatements(path)
		if err != nil {
			return fmt.Errorf("load %s fixture script: %w", backend, err)
		}
		for index, statement := range statements {
			if _, err := database.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf(
					"execute %s fixture statement %d: %w",
					backend,
					index+1,
					err,
				)
			}
		}
	}
	return nil
}

// QuerySQLRecords reads the stable fixture in deterministic ID order.
func QuerySQLRecords(
	ctx context.Context,
	database *sql.DB,
) ([]fixture.SQLRecord, error) {
	if ctx == nil {
		return nil, fmt.Errorf("query SQL fixture: nil context")
	}
	if database == nil {
		return nil, fmt.Errorf("query SQL fixture: nil database handle")
	}
	rows, err := database.QueryContext(ctx, `
SELECT
    id,
    number_value,
    decimal_value,
    text_value,
    bool_value,
    created_at,
    nullable_number,
    nullable_text,
    equality_only_text
FROM semantic_records
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query SQL fixture records: %w", err)
	}
	defer rows.Close()

	records := make([]fixture.SQLRecord, 0, len(fixture.StableIDs()))
	for rows.Next() {
		var record fixture.SQLRecord
		if err := rows.Scan(
			&record.ID,
			&record.Number,
			&record.Decimal,
			&record.Text,
			&record.Boolean,
			&record.CreatedAt,
			&record.NullableNumber,
			&record.NullableText,
			&record.EqualityOnlyText,
		); err != nil {
			return nil, fmt.Errorf("scan SQL fixture record: %w", err)
		}
		record.CreatedAt = record.CreatedAt.UTC()
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate SQL fixture records: %w", err)
	}
	return records, nil
}

func readSQLStatements(path string) ([]string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	statements := splitSQLStatements(string(contents))
	if len(statements) == 0 {
		return nil, fmt.Errorf("script contains no statements")
	}
	return statements, nil
}

func splitSQLStatements(contents string) []string {
	statements := make([]string, 0, 2)
	var current strings.Builder
	flush := func() {
		statement := strings.TrimSpace(current.String())
		if statement != "" {
			statements = append(statements, statement)
		}
		current.Reset()
	}
	for _, line := range strings.Split(contents, "\n") {
		if strings.TrimSpace(line) == scriptBoundary {
			flush()
			continue
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	flush()
	return statements
}
