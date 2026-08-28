// Package fixture defines stable service records and comparison helpers used
// by the runnable environment and integration checks.
package fixture

import (
	"database/sql"
	"fmt"
	"slices"
	"time"
)

var stableIDs = []string{"r01", "r02", "r03", "r04", "r05", "r06"}

// SQLRecord is the common row shape materialized by both SQL backends.
type SQLRecord struct {
	ID               string
	Number           int64
	Decimal          string
	Text             string
	Boolean          bool
	CreatedAt        time.Time
	NullableNumber   sql.NullInt64
	NullableText     sql.NullString
	EqualityOnlyText string
}

// StableIDs returns a fresh copy of the fixture record identifiers.
func StableIDs() []string {
	return append([]string(nil), stableIDs...)
}

// IDs returns the record identifiers in their current order.
func IDs(records []SQLRecord) []string {
	ids := make([]string, len(records))
	for index := range records {
		ids[index] = records[index].ID
	}
	return ids
}

// CompareIDs compares two identifier sets without depending on result order.
func CompareIDs(got, want []string) error {
	gotCanonical, err := CanonicalIDs(got)
	if err != nil {
		return fmt.Errorf("received IDs: %w", err)
	}
	wantCanonical, err := CanonicalIDs(want)
	if err != nil {
		return fmt.Errorf("expected IDs: %w", err)
	}
	if !slices.Equal(gotCanonical, wantCanonical) {
		return fmt.Errorf(
			"ID set mismatch: received %v, expected %v",
			gotCanonical,
			wantCanonical,
		)
	}
	return nil
}

// CanonicalIDs validates IDs, removes duplicates, and returns a sorted copy.
func CanonicalIDs(ids []string) ([]string, error) {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("empty record ID")
		}
		set[id] = struct{}{}
	}
	canonical := make([]string, 0, len(set))
	for id := range set {
		canonical = append(canonical, id)
	}
	slices.Sort(canonical)
	return canonical, nil
}

// CompareSQLRecords verifies that two backends materialize the same fixture.
// Errors identify only the stable record ID and differing column, never the
// stored value.
func CompareSQLRecords(left, right []SQLRecord) error {
	if err := CompareIDs(IDs(left), IDs(right)); err != nil {
		return err
	}

	rightByID := make(map[string]SQLRecord, len(right))
	for _, record := range right {
		if _, exists := rightByID[record.ID]; exists {
			return fmt.Errorf("duplicate record ID %q in right fixture", record.ID)
		}
		rightByID[record.ID] = record
	}
	seen := make(map[string]struct{}, len(left))
	for _, record := range left {
		if _, exists := seen[record.ID]; exists {
			return fmt.Errorf("duplicate record ID %q in left fixture", record.ID)
		}
		seen[record.ID] = struct{}{}
		other := rightByID[record.ID]
		if field := differingField(record, other); field != "" {
			return fmt.Errorf("record %q differs in column %s", record.ID, field)
		}
	}
	return nil
}

func differingField(left, right SQLRecord) string {
	switch {
	case left.Number != right.Number:
		return "number_value"
	case left.Decimal != right.Decimal:
		return "decimal_value"
	case left.Text != right.Text:
		return "text_value"
	case left.Boolean != right.Boolean:
		return "bool_value"
	case !left.CreatedAt.Equal(right.CreatedAt):
		return "created_at"
	case left.NullableNumber != right.NullableNumber:
		return "nullable_number"
	case left.NullableText != right.NullableText:
		return "nullable_text"
	case left.EqualityOnlyText != right.EqualityOnlyText:
		return "equality_only_text"
	default:
		return ""
	}
}
