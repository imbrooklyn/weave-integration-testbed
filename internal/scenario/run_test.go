package scenario

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompareReferenceAllowsDeclaredSkippedScenario(t *testing.T) {
	reference := Report{
		Adapter: "memory",
		Backend: "reference",
		Results: []Result{
			{Name: "one", IDs: []string{"r02", "r01"}},
			{Name: "two", IDs: []string{"r03"}},
		},
	}
	candidate := Report{
		Adapter: "gorm",
		Backend: "mysql",
		Results: []Result{{Name: "one", IDs: []string{"r01", "r02"}}},
		Skipped: []string{"two"},
	}
	if err := CompareReference(reference, candidate); err != nil {
		t.Fatalf("CompareReference() error = %v", err)
	}
}

func TestCompareReferenceRejectsMatchSetDifference(t *testing.T) {
	reference := Report{Results: []Result{{Name: "one", IDs: []string{"r01"}}}}
	candidate := Report{Results: []Result{{Name: "one", IDs: []string{"r02"}}}}
	if err := CompareReference(reference, candidate); err == nil {
		t.Fatal("CompareReference() error = nil")
	}
}

func TestCompareReferenceAllowsCanonicalMissingCollapse(t *testing.T) {
	reference := Report{Results: []Result{{Name: "null", IDs: []string{"r03"}}}}
	candidate := Report{Results: []Result{{
		Name:                    "null",
		IDs:                     []string{"r03", "r04"},
		UsesMissingCollapsedSet: true,
	}}}
	if err := CompareReference(reference, candidate); err != nil {
		t.Fatalf("CompareReference() error = %v", err)
	}
}

func TestCompareEquivalentIncludesStorageAdjustedSets(t *testing.T) {
	left := Report{
		Results: []Result{{
			Name:                    "null",
			IDs:                     []string{"r03", "r04"},
			UsesMissingCollapsedSet: true,
		}},
		Skipped: []string{"missing"},
	}
	right := Report{
		Results: []Result{{
			Name:                    "null",
			IDs:                     []string{"r04", "r03"},
			UsesMissingCollapsedSet: true,
		}},
		Skipped: []string{"missing"},
	}
	if err := CompareEquivalent(left, right); err != nil {
		t.Fatalf("CompareEquivalent() error = %v", err)
	}
}

func TestWriteReportUsesStableMatchSetEvidence(t *testing.T) {
	report := Report{
		Adapter: "gormgen",
		Backend: "postgres",
		Results: []Result{{Name: "scalar equality", IDs: []string{"r03"}}},
		Skipped: []string{"missing state"},
	}
	var output bytes.Buffer
	if err := WriteReport(&output, report); err != nil {
		t.Fatalf("WriteReport() error = %v", err)
	}
	for _, want := range []string{
		"gormgen/postgres: 1 passed, 1 skipped",
		"scalar equality => [r03]",
		"missing state => skipped",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("WriteReport() output does not contain %q: %s", want, output.String())
		}
	}
}
