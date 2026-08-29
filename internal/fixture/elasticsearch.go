package fixture

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/imbrooklyn/weave/compilertest"
)

// ElasticsearchIndex is the default canonical search fixture index.
const ElasticsearchIndex = "weave-semantic-records"

// ElasticsearchFiles contains the three committed search fixture payloads.
type ElasticsearchFiles struct {
	Settings json.RawMessage
	Mapping  json.RawMessage
	Bulk     []byte
}

// LoadElasticsearchFiles reads and structurally validates the committed
// settings, explicit mapping, and NDJSON bulk fixture beneath root.
func LoadElasticsearchFiles(root string) (ElasticsearchFiles, error) {
	directory := filepath.Join(root, "testdata", "elasticsearch")
	settings, err := readJSONFile(filepath.Join(directory, "settings.json"))
	if err != nil {
		return ElasticsearchFiles{}, err
	}
	mapping, err := readJSONFile(filepath.Join(directory, "mapping.json"))
	if err != nil {
		return ElasticsearchFiles{}, err
	}
	bulk, err := os.ReadFile(filepath.Join(directory, "records.ndjson"))
	if err != nil {
		return ElasticsearchFiles{}, fmt.Errorf("read Elasticsearch bulk fixture")
	}
	if err := ValidateElasticsearchBulk(bulk); err != nil {
		return ElasticsearchFiles{}, err
	}
	return ElasticsearchFiles{
		Settings: settings,
		Mapping:  mapping,
		Bulk:     append([]byte(nil), bulk...),
	}, nil
}

// ValidateElasticsearchBulk proves that the committed NDJSON materializes the
// canonical compilertest records without duplicating their semantic case list.
func ValidateElasticsearchBulk(data []byte) error {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	canonical := compilertest.Records()
	byID := make(map[string]compilertest.Record, len(canonical))
	for _, record := range canonical {
		byID[record.ID] = record
	}

	seen := make([]string, 0, len(canonical))
	line := 0
	for scanner.Scan() {
		line++
		actionLine := append([]byte(nil), scanner.Bytes()...)
		if !scanner.Scan() {
			return fmt.Errorf("Elasticsearch bulk fixture has an unmatched action line")
		}
		line++
		sourceLine := append([]byte(nil), scanner.Bytes()...)

		var action struct {
			Index struct {
				ID string `json:"_id"`
			} `json:"index"`
		}
		if err := json.Unmarshal(actionLine, &action); err != nil || action.Index.ID == "" {
			return fmt.Errorf("Elasticsearch bulk fixture action is invalid at line %d", line-1)
		}
		record, ok := byID[action.Index.ID]
		if !ok || slices.Contains(seen, action.Index.ID) {
			return fmt.Errorf("Elasticsearch bulk fixture ID contract is invalid")
		}
		var source map[string]json.RawMessage
		if err := json.Unmarshal(sourceLine, &source); err != nil || source == nil {
			return fmt.Errorf("Elasticsearch bulk fixture source is invalid at line %d", line)
		}
		if err := compareElasticsearchCanonicalSource(record, source); err != nil {
			return fmt.Errorf("Elasticsearch bulk fixture record %q: %w", record.ID, err)
		}
		seen = append(seen, record.ID)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan Elasticsearch bulk fixture")
	}
	if err := CompareIDs(seen, StableIDs()); err != nil {
		return fmt.Errorf("Elasticsearch bulk fixture: %w", err)
	}
	return nil
}

func compareElasticsearchCanonicalSource(
	record compilertest.Record,
	source map[string]json.RawMessage,
) error {
	if !rawEquals(source["number_value"], record.Number) ||
		!rawEquals(source["text_value"], record.Text) ||
		!rawEquals(source["equality_only_text"], record.Text) {
		return fmt.Errorf("canonical scalar fields differ")
	}
	if !rawEquals(source["text_value_state"], "value") ||
		!rawEquals(source["equality_only_text_state"], "value") {
		return fmt.Errorf("required companion state differs")
	}
	if err := compareNullableRaw(
		source, "nullable_number", "", record.NullableNumberPresent,
		record.NullableNumber,
	); err != nil {
		return err
	}
	if err := compareNullableRaw(
		source, "nullable_text", "nullable_text_state",
		record.NullableTextPresent, record.NullableText,
	); err != nil {
		return err
	}
	return nil
}

func compareNullableRaw[T any](
	source map[string]json.RawMessage,
	field string,
	marker string,
	present bool,
	value *T,
) error {
	raw, exists := source[field]
	if exists != present {
		return fmt.Errorf("nullable field presence differs")
	}
	if !present {
		if marker != "" {
			if _, markerExists := source[marker]; markerExists {
				return fmt.Errorf("missing nullable field has a companion marker")
			}
		}
		return nil
	}
	if value == nil {
		if string(raw) != "null" {
			return fmt.Errorf("explicit null field differs")
		}
		if marker != "" && !rawEquals(source[marker], "null") {
			return fmt.Errorf("explicit null companion marker differs")
		}
		return nil
	}
	if !rawEquals(raw, *value) {
		return fmt.Errorf("nullable field value differs")
	}
	if marker != "" && !rawEquals(source[marker], "value") {
		return fmt.Errorf("value companion marker differs")
	}
	return nil
}

func rawEquals[T any](raw json.RawMessage, want T) bool {
	if len(raw) == 0 {
		return false
	}
	var got T
	return json.Unmarshal(raw, &got) == nil && reflect.DeepEqual(got, want)
}

func readJSONFile(path string) (json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Elasticsearch JSON fixture")
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("Elasticsearch JSON fixture is invalid")
	}
	if strings.TrimSpace(string(data)) == "null" {
		return nil, fmt.Errorf("Elasticsearch JSON fixture is null")
	}
	return append(json.RawMessage(nil), data...), nil
}
