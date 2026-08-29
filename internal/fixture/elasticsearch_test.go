package fixture

import "testing"

func TestElasticsearchFilesMatchCanonicalFixture(t *testing.T) {
	files, err := LoadElasticsearchFiles("../..")
	if err != nil {
		t.Fatalf("LoadElasticsearchFiles: %v", err)
	}
	if len(files.Settings) == 0 || len(files.Mapping) == 0 || len(files.Bulk) == 0 {
		t.Fatal("Elasticsearch fixture contains an empty payload")
	}
}
