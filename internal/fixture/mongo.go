package fixture

import (
	"github.com/imbrooklyn/weave/compilertest"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// MongoCollection is the collection containing the canonical compiler fixture.
const MongoCollection = "semantic_records"

// MongoRegexProbeCollection contains server-only literal-regex seam records.
const MongoRegexProbeCollection = "regex_probe_records"

// MongoInjectionText is a query-like value that must remain a BSON string.
const MongoInjectionText = `.*"}],"$where":"return true"`

// MongoRecords materializes compilertest.Records as independent ordered BSON
// documents while preserving value, explicit-null, and missing field states.
func MongoRecords() []bson.D {
	records := compilertest.Records()
	documents := make([]bson.D, len(records))
	for index, record := range records {
		document := bson.D{
			{Key: "_id", Value: record.ID},
			{Key: "number_value", Value: record.Number},
			{Key: "text_value", Value: record.Text},
		}
		if record.NullableNumberPresent {
			var value any
			if record.NullableNumber != nil {
				value = *record.NullableNumber
			}
			document = append(document, bson.E{Key: "nullable_number", Value: value})
		}
		if record.NullableTextPresent {
			var value any
			if record.NullableText != nil {
				value = *record.NullableText
			}
			document = append(document, bson.E{Key: "nullable_text", Value: value})
		}
		document = append(document, bson.E{
			Key:   "equality_only_text",
			Value: record.Text,
		})
		documents[index] = document
	}
	return documents
}

// MongoRegexProbeRecords returns independent ordered documents for real-server
// checks of literal metacharacters, backslashes, Unicode, and absolute anchors.
func MongoRegexProbeRecords() []bson.D {
	return []bson.D{
		{{Key: "_id", Value: "p01"}, {Key: "text_value", Value: "alpha\n"}},
		{{Key: "_id", Value: "p02"}, {Key: "text_value", Value: "alpha"}},
		{{Key: "_id", Value: "p03"}, {Key: "text_value", Value: "literal.*\\世界\nend"}},
		{{Key: "_id", Value: "p04"}, {Key: "text_value", Value: "x\nliteral.*\\世界\nend"}},
		{{Key: "_id", Value: "p05"}, {Key: "text_value", Value: "literal.*\\世界\nend\n"}},
		{{Key: "_id", Value: "p06"}, {Key: "text_value", Value: MongoInjectionText}},
	}
}
