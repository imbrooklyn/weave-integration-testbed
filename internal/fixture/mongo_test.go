package fixture

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMongoRecordsPreserveCanonicalThreeStateFixture(t *testing.T) {
	documents := MongoRecords()
	if got, want := len(documents), len(StableIDs()); got != want {
		t.Fatalf("MongoRecords() count = %d, want %d", got, want)
	}
	byID := mongoDocumentsByID(t, documents)
	assertMongoFieldState(t, byID["r01"], "nullable_number", true, false)
	assertMongoFieldState(t, byID["r03"], "nullable_number", true, true)
	assertMongoFieldState(t, byID["r04"], "nullable_number", false, false)
	assertMongoFieldState(t, byID["r01"], "nullable_text", true, false)
	assertMongoFieldState(t, byID["r03"], "nullable_text", true, true)
	assertMongoFieldState(t, byID["r04"], "nullable_text", false, false)

	documents[0][0].Value = "changed"
	if got := MongoRecords()[0][0].Value; got != "r01" {
		t.Fatalf("MongoRecords() shares mutable document storage: %v", got)
	}
}

func TestCommittedMongoInitialDataMatchesGeneratedFixtures(t *testing.T) {
	tests := []struct {
		name string
		want []bson.D
	}{
		{name: "records.json", want: MongoRecords()},
		{name: "regex_records.json", want: MongoRegexProbeRecords()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join("..", "..", "testdata", "mongo", test.name)
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			var got []bson.D
			if err := bson.UnmarshalExtJSON(contents, false, &got); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("%s does not match its generated fixture", path)
			}
		})
	}
}

func mongoDocumentsByID(t *testing.T, documents []bson.D) map[string]bson.D {
	t.Helper()
	byID := make(map[string]bson.D, len(documents))
	for _, document := range documents {
		if len(document) == 0 || document[0].Key != "_id" {
			t.Fatal("Mongo fixture document does not start with _id")
		}
		id, ok := document[0].Value.(string)
		if !ok || id == "" {
			t.Fatal("Mongo fixture document has invalid _id")
		}
		if _, exists := byID[id]; exists {
			t.Fatalf("Mongo fixture contains duplicate _id %q", id)
		}
		byID[id] = document
	}
	return byID
}

func assertMongoFieldState(
	t *testing.T,
	document bson.D,
	field string,
	wantPresent bool,
	wantNull bool,
) {
	t.Helper()
	found := false
	var value any
	for _, element := range document {
		if element.Key == field {
			found = true
			value = element.Value
			break
		}
	}
	if found != wantPresent || (found && (value == nil) != wantNull) {
		t.Fatalf(
			"field %q state = (present=%v, null=%v), want (%v, %v)",
			field,
			found,
			found && value == nil,
			wantPresent,
			wantNull,
		)
	}
}
