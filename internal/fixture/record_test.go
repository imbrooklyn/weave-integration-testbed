package fixture

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestStableIDsReturnsIndependentCopy(t *testing.T) {
	first := StableIDs()
	first[0] = "changed"
	second := StableIDs()
	if second[0] != "r01" {
		t.Fatalf("StableIDs() leaked mutable storage: %v", second)
	}
}

func TestCompareIDsUsesSetSemantics(t *testing.T) {
	if err := CompareIDs([]string{"r02", "r01", "r02"}, []string{"r01", "r02"}); err != nil {
		t.Fatalf("CompareIDs() error = %v", err)
	}
}

func TestCompareSQLRecordsDoesNotExposeValues(t *testing.T) {
	left := testRecord("left secret")
	right := testRecord("right secret")
	err := CompareSQLRecords([]SQLRecord{left}, []SQLRecord{right})
	if err == nil {
		t.Fatal("CompareSQLRecords() error = nil")
	}
	for _, secret := range []string{"left secret", "right secret"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("CompareSQLRecords() error exposes a value: %v", err)
		}
	}
}

func testRecord(text string) SQLRecord {
	return SQLRecord{
		ID:               "r01",
		Number:           1,
		Decimal:          "1.125",
		Text:             text,
		Boolean:          true,
		CreatedAt:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		NullableNumber:   sql.NullInt64{Int64: 1, Valid: true},
		NullableText:     sql.NullString{String: text, Valid: true},
		EqualityOnlyText: text,
	}
}
