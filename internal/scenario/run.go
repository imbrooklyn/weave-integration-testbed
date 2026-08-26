// Package scenario executes and reports the canonical compilertest match-set
// scenarios without depending on testing.T.
package scenario

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/imbrooklyn/weave-integration-testbed/internal/fixture"
	"github.com/imbrooklyn/weave/compilertest"
)

// Result is one successful canonical scenario execution.
type Result struct {
	Name                    string
	IDs                     []string
	UsesMissingCollapsedSet bool
}

// Report is the stable match-set evidence for one Adapter and backend pair.
type Report struct {
	Adapter string
	Backend string
	Results []Result
	Skipped []string
}

// Run compiles and executes every applicable canonical scenario, compares the
// final ID set with compilertest's expected IDs, and returns a stable report.
func Run[C, E any](
	ctx context.Context,
	adapter string,
	backend string,
	harness compilertest.Harness[C, E],
) (Report, error) {
	report := Report{Adapter: adapter, Backend: backend}
	if ctx == nil {
		return report, fmt.Errorf("%s/%s scenarios: nil context", adapter, backend)
	}
	if strings.TrimSpace(adapter) == "" || strings.TrimSpace(backend) == "" {
		return report, fmt.Errorf("scenario report identity must not be blank")
	}
	if harness.Factory == nil {
		return report, fmt.Errorf("%s/%s scenarios: nil Factory", adapter, backend)
	}
	if harness.Execute == nil {
		return report, fmt.Errorf("%s/%s scenarios: nil Execute callback", adapter, backend)
	}

	for _, canonical := range compilertest.Scenarios(harness) {
		if err := ctx.Err(); err != nil {
			return report, fmt.Errorf("%s/%s scenarios: %w", adapter, backend, err)
		}
		if canonical.RequiresDistinctMissing() && !harness.DistinguishesMissing {
			report.Skipped = append(report.Skipped, canonical.Name())
			continue
		}

		condition, err := canonical.Build(harness.Factory)
		if err != nil {
			return report, fmt.Errorf(
				"%s/%s scenario %q build: %w",
				adapter,
				backend,
				canonical.Name(),
				err,
			)
		}
		if harness.InspectCondition != nil {
			if err := harness.InspectCondition(canonical.Name(), condition); err != nil {
				return report, fmt.Errorf(
					"%s/%s scenario %q inspect: %w",
					adapter,
					backend,
					canonical.Name(),
					err,
				)
			}
		}
		ids, err := harness.Execute(condition)
		if err != nil {
			return report, fmt.Errorf(
				"%s/%s scenario %q execute: %w",
				adapter,
				backend,
				canonical.Name(),
				err,
			)
		}
		if err := fixture.CompareIDs(ids, canonical.ExpectedIDs()); err != nil {
			return report, fmt.Errorf(
				"%s/%s scenario %q: %w",
				adapter,
				backend,
				canonical.Name(),
				err,
			)
		}
		canonicalIDs, err := fixture.CanonicalIDs(ids)
		if err != nil {
			return report, fmt.Errorf(
				"%s/%s scenario %q IDs: %w",
				adapter,
				backend,
				canonical.Name(),
				err,
			)
		}
		report.Results = append(report.Results, Result{
			Name:                    canonical.Name(),
			IDs:                     canonicalIDs,
			UsesMissingCollapsedSet: canonical.UsesMissingCollapsedMatchSet(),
		})
	}
	return report, nil
}

// CompareReference verifies that candidate contains the same result for every
// scenario it can represent and accounts for every reference scenario.
func CompareReference(reference, candidate Report) error {
	if len(reference.Skipped) != 0 {
		return fmt.Errorf("reference report contains skipped scenarios")
	}
	referenceByName := make(map[string][]string, len(reference.Results))
	for _, result := range reference.Results {
		if _, exists := referenceByName[result.Name]; exists {
			return fmt.Errorf("reference report contains duplicate scenario %q", result.Name)
		}
		referenceByName[result.Name] = result.IDs
	}

	seen := make(map[string]struct{}, len(candidate.Results)+len(candidate.Skipped))
	for _, result := range candidate.Results {
		want, exists := referenceByName[result.Name]
		if !exists {
			return fmt.Errorf("candidate report contains unknown scenario %q", result.Name)
		}
		if _, duplicate := seen[result.Name]; duplicate {
			return fmt.Errorf("candidate report contains duplicate scenario %q", result.Name)
		}
		seen[result.Name] = struct{}{}
		if !result.UsesMissingCollapsedSet {
			if err := fixture.CompareIDs(result.IDs, want); err != nil {
				return fmt.Errorf("scenario %q differs from reference: %w", result.Name, err)
			}
		}
	}
	for _, name := range candidate.Skipped {
		if _, exists := referenceByName[name]; !exists {
			return fmt.Errorf("candidate report skips unknown scenario %q", name)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("candidate report repeats scenario %q", name)
		}
		seen[name] = struct{}{}
	}
	if len(seen) != len(referenceByName) {
		return fmt.Errorf(
			"candidate report accounts for %d scenarios, want %d",
			len(seen),
			len(referenceByName),
		)
	}
	return nil
}

// CompareEquivalent verifies that two reports for the same storage semantics
// contain identical scenario names, skip decisions, adjustment metadata, and
// final ID sets.
func CompareEquivalent(left, right Report) error {
	leftByName := make(map[string]Result, len(left.Results))
	for _, result := range left.Results {
		if _, duplicate := leftByName[result.Name]; duplicate {
			return fmt.Errorf("left report contains duplicate scenario %q", result.Name)
		}
		leftByName[result.Name] = result
	}
	if len(left.Results) != len(right.Results) {
		return fmt.Errorf(
			"report result counts differ: %d != %d",
			len(left.Results),
			len(right.Results),
		)
	}
	seen := make(map[string]struct{}, len(right.Results))
	for _, result := range right.Results {
		want, exists := leftByName[result.Name]
		if !exists {
			return fmt.Errorf("right report contains unknown scenario %q", result.Name)
		}
		if _, duplicate := seen[result.Name]; duplicate {
			return fmt.Errorf("right report contains duplicate scenario %q", result.Name)
		}
		seen[result.Name] = struct{}{}
		if result.UsesMissingCollapsedSet != want.UsesMissingCollapsedSet {
			return fmt.Errorf("scenario %q storage-adjustment metadata differs", result.Name)
		}
		if err := fixture.CompareIDs(result.IDs, want.IDs); err != nil {
			return fmt.Errorf("scenario %q differs from reference: %w", result.Name, err)
		}
	}
	leftSkipped := make(map[string]struct{}, len(left.Skipped))
	for _, name := range left.Skipped {
		leftSkipped[name] = struct{}{}
	}
	if len(left.Skipped) != len(right.Skipped) {
		return fmt.Errorf(
			"report skipped counts differ: %d != %d",
			len(left.Skipped),
			len(right.Skipped),
		)
	}
	for _, name := range right.Skipped {
		if _, exists := leftSkipped[name]; !exists {
			return fmt.Errorf("right report skips a different scenario %q", name)
		}
	}
	return nil
}

// WriteReport writes deterministic English match-set evidence without query
// values, connection strings, or backend condition text.
func WriteReport(writer io.Writer, report Report) error {
	if writer == nil {
		return fmt.Errorf("write scenario report: nil writer")
	}
	if _, err := fmt.Fprintf(
		writer,
		"%s/%s: %d passed, %d skipped\n",
		report.Adapter,
		report.Backend,
		len(report.Results),
		len(report.Skipped),
	); err != nil {
		return err
	}
	for _, result := range report.Results {
		adjustment := ""
		if result.UsesMissingCollapsedSet {
			adjustment = " (missing materialized as null)"
		}
		if _, err := fmt.Fprintf(
			writer,
			"  %s => %v%s\n",
			result.Name,
			result.IDs,
			adjustment,
		); err != nil {
			return err
		}
	}
	for _, name := range report.Skipped {
		if _, err := fmt.Fprintf(
			writer,
			"  %s => skipped (missing is materialized as null)\n",
			name,
		); err != nil {
			return err
		}
	}
	return nil
}
