// Package demoapp provides finite command execution and stable exit behavior
// for the runnable Adapter demos.
package demoapp

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/imbrooklyn/weave-integration-testbed/internal/scenario"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
)

const defaultTimeout = 2 * time.Minute

// MemoryRunner executes the memory reference demo.
type MemoryRunner func(context.Context) (scenario.Report, error)

// SQLRunner executes one SQL Adapter/backend pair.
type SQLRunner func(context.Context, testenv.Backend) (scenario.Report, error)

// RunMemory parses the common timeout, executes runner, and returns a process
// exit code: zero for success, one for execution failure, and two for usage.
func RunMemory(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	runner MemoryRunner,
) int {
	flags := flag.NewFlagSet("memory", flag.ContinueOnError)
	flags.SetOutput(stderr)
	timeout := flags.Duration("timeout", defaultTimeout, "overall demo timeout")
	if code := parseFlags(flags, args, timeout, stderr); code >= 0 {
		return code
	}
	if runner == nil {
		fmt.Fprintln(stderr, "memory demo failed: nil runner")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	report, err := runner(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "memory demo failed: %v\n", err)
		return 1
	}
	if err := scenario.WriteReport(stdout, report); err != nil {
		fmt.Fprintf(stderr, "memory demo failed to write report: %v\n", err)
		return 1
	}
	return 0
}

// RunSQL parses a backend selection and finite per-backend timeout, executes
// runner sequentially, and returns a stable process exit code.
func RunSQL(
	adapter string,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	runner SQLRunner,
) int {
	flags := flag.NewFlagSet(adapter, flag.ContinueOnError)
	flags.SetOutput(stderr)
	timeout := flags.Duration("timeout", defaultTimeout, "per-backend demo timeout")
	backendName := flags.String("backend", "all", "all, mysql, or postgres")
	if code := parseFlags(flags, args, timeout, stderr); code >= 0 {
		return code
	}
	backends, err := parseBackends(*backendName)
	if err != nil {
		fmt.Fprintf(stderr, "%s demo usage error: %v\n", adapter, err)
		return 2
	}
	if runner == nil {
		fmt.Fprintf(stderr, "%s demo failed: nil runner\n", adapter)
		return 1
	}

	for _, backend := range backends {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		report, runErr := runner(ctx, backend)
		cancel()
		if runErr != nil {
			fmt.Fprintf(stderr, "%s/%s demo failed: %v\n", adapter, backend, runErr)
			return 1
		}
		if err := scenario.WriteReport(stdout, report); err != nil {
			fmt.Fprintf(stderr, "%s/%s demo failed to write report: %v\n", adapter, backend, err)
			return 1
		}
	}
	return 0
}

func parseFlags(
	flags *flag.FlagSet,
	args []string,
	timeout *time.Duration,
	stderr io.Writer,
) int {
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "%s does not accept positional arguments\n", flags.Name())
		return 2
	}
	if timeout == nil || *timeout <= 0 {
		fmt.Fprintf(stderr, "%s timeout must be positive\n", flags.Name())
		return 2
	}
	return -1
}

func parseBackends(value string) ([]testenv.Backend, error) {
	if strings.EqualFold(strings.TrimSpace(value), "all") {
		return testenv.SQLBackends(), nil
	}
	backend, err := testenv.ParseBackend(value)
	if err != nil {
		return nil, err
	}
	return []testenv.Backend{backend}, nil
}
