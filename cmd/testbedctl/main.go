// Command testbedctl controls the local SQL fixture environment.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/imbrooklyn/weave-integration-testbed/internal/fixture"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
)

const defaultTimeout = 2 * time.Minute

type options struct {
	backend string
	root    string
	timeout time.Duration
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "testbedctl: %v\n", err)
		os.Exit(1)
	}
}

func run(arguments []string, output io.Writer) error {
	if len(arguments) == 0 {
		return fmt.Errorf("expected one command: health, reset, verify, or check")
	}
	command := arguments[0]
	switch command {
	case "health", "reset", "verify", "check":
	default:
		return fmt.Errorf("unsupported command %q", command)
	}

	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	config := options{}
	flags.StringVar(&config.backend, "backend", "all", "SQL backend: all, mysql, or postgres")
	flags.StringVar(&config.root, "root", ".", "repository root containing testdata")
	flags.DurationVar(&config.timeout, "timeout", defaultTimeout, "timeout per SQL backend")
	if err := flags.Parse(arguments[1:]); err != nil {
		return fmt.Errorf("parse %s options: %w", command, err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments for %s", command)
	}
	if config.timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	backends, err := selectedBackends(config.backend)
	if err != nil {
		return err
	}

	recordsByBackend := make(map[testenv.Backend][]fixture.SQLRecord, len(backends))
	var failures []error
	for _, backend := range backends {
		records, runErr := runForBackend(command, config, backend, output)
		if runErr != nil {
			failures = append(failures, runErr)
			continue
		}
		if records != nil {
			recordsByBackend[backend] = records
		}
	}
	if len(failures) != 0 {
		return errors.Join(failures...)
	}
	if command == "check" && len(backends) == 2 {
		if err := fixture.CompareSQLRecords(
			recordsByBackend[testenv.MySQL],
			recordsByBackend[testenv.PostgreSQL],
		); err != nil {
			return fmt.Errorf("compare SQL fixtures: %w", err)
		}
		fmt.Fprintln(output, "sql: MySQL and PostgreSQL fixtures match")
	}
	return nil
}

func runForBackend(
	command string,
	options options,
	backend testenv.Backend,
	output io.Writer,
) (records []fixture.SQLRecord, resultErr error) {
	config, err := testenv.LoadSQLConfig(backend)
	if err != nil {
		return nil, fmt.Errorf("load %s configuration: %w", backend, err)
	}
	database, err := testenv.OpenSQL(config)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", backend, err)
	}
	defer func() {
		if err := database.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close %s database: %w", backend, err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), options.timeout)
	defer cancel()
	if err := testenv.WaitForSQL(ctx, backend, database, 500*time.Millisecond); err != nil {
		return nil, err
	}

	switch command {
	case "health":
		fmt.Fprintf(output, "%s: healthy\n", backend)
		return nil, nil
	case "reset":
		if err := testenv.ResetSQL(ctx, database, options.root, backend); err != nil {
			return nil, err
		}
		fmt.Fprintf(output, "%s: fixture reset\n", backend)
		return nil, nil
	case "verify":
		records, err = verifyFixture(ctx, database)
		if err != nil {
			return nil, fmt.Errorf("verify %s fixture: %w", backend, err)
		}
		fmt.Fprintf(output, "%s: fixture IDs verified (%d records)\n", backend, len(records))
		return records, nil
	case "check":
		if err := testenv.ResetSQL(ctx, database, options.root, backend); err != nil {
			return nil, err
		}
		records, err = verifyFixture(ctx, database)
		if err != nil {
			return nil, fmt.Errorf("verify %s fixture: %w", backend, err)
		}
		fmt.Fprintf(output, "%s: fixture reset and verified (%d records)\n", backend, len(records))
		return records, nil
	default:
		return nil, fmt.Errorf("unsupported command %q", command)
	}
}

func verifyFixture(
	ctx context.Context,
	database *sql.DB,
) ([]fixture.SQLRecord, error) {
	records, err := testenv.QuerySQLRecords(ctx, database)
	if err != nil {
		return nil, err
	}
	if err := fixture.CompareIDs(fixture.IDs(records), fixture.StableIDs()); err != nil {
		return nil, err
	}
	return records, nil
}

func selectedBackends(value string) ([]testenv.Backend, error) {
	if value == "all" {
		return testenv.SQLBackends(), nil
	}
	backend, err := testenv.ParseBackend(value)
	if err != nil {
		return nil, err
	}
	return []testenv.Backend{backend}, nil
}
