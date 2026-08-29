package testenv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	goelasticsearch "github.com/elastic/go-elasticsearch/v9"
	"github.com/imbrooklyn/weave-adapters/elasticsearch"
	"github.com/imbrooklyn/weave-integration-testbed/internal/fixture"
)

const (
	defaultElasticsearchHost    = "127.0.0.1"
	defaultElasticsearchPort    = uint16(39200)
	defaultElasticsearchVersion = "9.5.2"
	defaultLuceneVersion        = "10.5.1"
)

// ElasticsearchConfig contains the loopback endpoint and explicit fixture
// index used by the search profile.
type ElasticsearchConfig struct {
	Host  string
	Port  uint16
	Index string
}

// ElasticsearchServerInfo is the verified non-secret server identity.
type ElasticsearchServerInfo struct {
	Version       string
	LuceneVersion string
}

// LoadElasticsearchConfig reads the documented search-profile environment
// variables and applies the same defaults as compose.yaml.
func LoadElasticsearchConfig() (ElasticsearchConfig, error) {
	port, err := environmentPort(
		"WEAVE_TESTBED_ELASTICSEARCH_PORT",
		defaultElasticsearchPort,
	)
	if err != nil {
		return ElasticsearchConfig{}, err
	}
	config := ElasticsearchConfig{
		Host: environmentValue(
			"WEAVE_TESTBED_ELASTICSEARCH_HOST",
			defaultElasticsearchHost,
		),
		Port: port,
		Index: environmentValue(
			"WEAVE_TESTBED_ELASTICSEARCH_INDEX",
			fixture.ElasticsearchIndex,
		),
	}
	if err := config.validate(); err != nil {
		return ElasticsearchConfig{}, err
	}
	return config, nil
}

// Endpoint returns the credential-free HTTP endpoint used by the local
// security-disabled search fixture.
func (config ElasticsearchConfig) Endpoint() string {
	return "http://" + net.JoinHostPort(
		config.Host,
		strconv.Itoa(int(config.Port)),
	)
}

// OpenElasticsearch creates the official typed client without contacting the
// server. Use WaitForElasticsearch for readiness and exact version checks.
func OpenElasticsearch(
	config ElasticsearchConfig,
) (*goelasticsearch.TypedClient, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	client, err := goelasticsearch.NewTypedClient(goelasticsearch.Config{
		Addresses:         []string{config.Endpoint()},
		DisableMetaHeader: false,
	})
	if err != nil {
		return nil, fmt.Errorf("open Elasticsearch typed client")
	}
	return client, nil
}

// WaitForElasticsearch retries the typed Info API until the server answers or
// ctx expires. Transport errors and response bodies are deliberately omitted.
func WaitForElasticsearch(
	ctx context.Context,
	client *goelasticsearch.TypedClient,
	interval time.Duration,
) error {
	if ctx == nil {
		return fmt.Errorf("wait for Elasticsearch: nil context")
	}
	if client == nil {
		return fmt.Errorf("wait for Elasticsearch: nil client")
	}
	if interval <= 0 {
		return fmt.Errorf("wait for Elasticsearch: interval must be positive")
	}
	for {
		if _, err := client.Info().Do(ctx); err == nil {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("Elasticsearch did not become healthy: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// ReadElasticsearchServerInfo verifies the exact server and Lucene versions
// locked by the search profile.
func ReadElasticsearchServerInfo(
	ctx context.Context,
	client *goelasticsearch.TypedClient,
) (ElasticsearchServerInfo, error) {
	if ctx == nil || client == nil {
		return ElasticsearchServerInfo{}, fmt.Errorf("read Elasticsearch server info: invalid input")
	}
	response, err := client.Info().Do(ctx)
	if err != nil {
		return ElasticsearchServerInfo{}, fmt.Errorf("read Elasticsearch server info")
	}
	if response.Version.Int != defaultElasticsearchVersion ||
		response.Version.LuceneVersion != defaultLuceneVersion {
		return ElasticsearchServerInfo{}, fmt.Errorf("Elasticsearch server is outside the locked 9.5.2 profile")
	}
	return ElasticsearchServerInfo{
		Version:       response.Version.Int,
		LuceneVersion: response.Version.LuceneVersion,
	}, nil
}

// ResetElasticsearch recreates the configured index from the committed
// settings, explicit mapping, and canonical NDJSON fixture beneath root.
func ResetElasticsearch(
	ctx context.Context,
	client *goelasticsearch.TypedClient,
	config ElasticsearchConfig,
	root string,
) error {
	if ctx == nil || client == nil {
		return fmt.Errorf("reset Elasticsearch fixture: invalid input")
	}
	files, err := fixture.LoadElasticsearchFiles(root)
	if err != nil {
		return err
	}
	indexPath := "/" + url.PathEscape(config.Index)
	if _, err := performElasticsearch(
		ctx, client, config, http.MethodDelete, indexPath, "", nil,
		http.StatusOK, http.StatusNotFound,
	); err != nil {
		return fmt.Errorf("delete Elasticsearch fixture index")
	}
	createBody, err := json.Marshal(struct {
		Settings json.RawMessage `json:"settings"`
		Mappings json.RawMessage `json:"mappings"`
	}{Settings: files.Settings, Mappings: files.Mapping})
	if err != nil {
		return fmt.Errorf("build Elasticsearch fixture index request")
	}
	if _, err := performElasticsearch(
		ctx, client, config, http.MethodPut, indexPath,
		"application/json", createBody, http.StatusOK,
	); err != nil {
		return fmt.Errorf("create Elasticsearch fixture index")
	}
	bulkPath := indexPath + "/_bulk?refresh=wait_for"
	responseBody, err := performElasticsearch(
		ctx, client, config, http.MethodPost, bulkPath,
		"application/x-ndjson", files.Bulk, http.StatusOK,
	)
	if err != nil {
		return fmt.Errorf("load Elasticsearch bulk fixture")
	}
	var bulkResponse struct {
		Errors bool `json:"errors"`
		Items  []map[string]struct {
			Status int `json:"status"`
		} `json:"items"`
	}
	if json.Unmarshal(responseBody, &bulkResponse) != nil ||
		bulkResponse.Errors || len(bulkResponse.Items) != len(fixture.StableIDs()) {
		return fmt.Errorf("Elasticsearch bulk fixture was not fully accepted")
	}
	for _, item := range bulkResponse.Items {
		for _, result := range item {
			if result.Status < 200 || result.Status >= 300 {
				return fmt.Errorf("Elasticsearch bulk fixture item failed")
			}
		}
	}
	return VerifyElasticsearchIndexContract(ctx, client, config)
}

// VerifyElasticsearchIndexContract checks the live explicit mapping and fixed
// index settings without performing mapping discovery for the Compiler.
func VerifyElasticsearchIndexContract(
	ctx context.Context,
	client *goelasticsearch.TypedClient,
	config ElasticsearchConfig,
) error {
	indexPath := "/" + url.PathEscape(config.Index)
	body, err := performElasticsearch(
		ctx, client, config, http.MethodGet, indexPath+"/_mapping",
		"", nil, http.StatusOK,
	)
	if err != nil {
		return fmt.Errorf("read Elasticsearch fixture mapping")
	}
	var response map[string]struct {
		Mappings struct {
			Dynamic    json.RawMessage `json:"dynamic"`
			Properties map[string]struct {
				Type       string          `json:"type"`
				Normalizer string          `json:"normalizer"`
				NullValue  json.RawMessage `json:"null_value"`
			} `json:"properties"`
		} `json:"mappings"`
	}
	if json.Unmarshal(body, &response) != nil || len(response) != 1 {
		return fmt.Errorf("decode Elasticsearch fixture mapping")
	}
	index, ok := response[config.Index]
	if !ok || string(index.Mappings.Dynamic) != `"strict"` {
		return fmt.Errorf("Elasticsearch fixture mapping is not strict")
	}
	expectedTypes := map[string]string{
		"number_value": "long", "text_value": "wildcard",
		"text_value_state": "keyword", "nullable_number": "long",
		"nullable_text": "wildcard", "nullable_text_state": "keyword",
		"equality_only_text":       "wildcard",
		"equality_only_text_state": "keyword", "decimal_value": "double",
		"created_at": "date", "bool_value": "boolean",
		"analyzed_text": "text", "normalized_keyword": "keyword",
		"expensive_keyword": "keyword", "pattern_wildcard": "wildcard",
		"tags": "keyword", "source_null": "keyword",
		"raw_null_keyword": "keyword", "untracked_keyword": "keyword",
		"empty_keyword": "keyword", "empty_array_keyword": "keyword",
		"nested_items": "nested",
	}
	for field, expectedType := range expectedTypes {
		if index.Mappings.Properties[field].Type != expectedType {
			return fmt.Errorf("Elasticsearch fixture field mapping differs")
		}
	}
	if index.Mappings.Properties["normalized_keyword"].Normalizer !=
		"weave_lowercase" ||
		len(index.Mappings.Properties["nullable_number"].NullValue) == 0 ||
		string(index.Mappings.Properties["raw_null_keyword"].NullValue) !=
			`"__WEAVE_NULL__"` {
		return fmt.Errorf("Elasticsearch fixture mapping options differ")
	}

	settingsBody, err := performElasticsearch(
		ctx, client, config, http.MethodGet,
		indexPath+"/_settings?flat_settings=true", "", nil, http.StatusOK,
	)
	if err != nil {
		return fmt.Errorf("read Elasticsearch fixture settings")
	}
	var settings map[string]struct {
		Settings map[string]json.RawMessage `json:"settings"`
	}
	if json.Unmarshal(settingsBody, &settings) != nil ||
		string(settings[config.Index].Settings["index.number_of_shards"]) != `"1"` ||
		string(settings[config.Index].Settings["index.number_of_replicas"]) != `"0"` ||
		string(settings[config.Index].Settings["index.analysis.normalizer.weave_lowercase.type"]) != `"custom"` {
		return fmt.Errorf("Elasticsearch fixture settings differ")
	}
	return nil
}

// QueryElasticsearchIDs executes one compiled typed Query and returns stable
// hit IDs. Query text and response bodies are omitted from errors.
func QueryElasticsearchIDs(
	ctx context.Context,
	client *goelasticsearch.TypedClient,
	config ElasticsearchConfig,
	query elasticsearch.Query,
) ([]string, error) {
	if ctx == nil || client == nil || query == nil {
		return nil, fmt.Errorf("query Elasticsearch fixture: invalid input")
	}
	response, err := client.Search().
		Index(config.Index).
		Query(query).
		Size(100).
		Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("execute Elasticsearch fixture query")
	}
	if response.TimedOut {
		return nil, fmt.Errorf("Elasticsearch fixture query timed out")
	}
	ids := make([]string, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		if hit.Id_ == nil || *hit.Id_ == "" {
			return nil, fmt.Errorf("decode Elasticsearch fixture ID")
		}
		ids = append(ids, *hit.Id_)
	}
	slices.Sort(ids)
	return ids, nil
}

// SetElasticsearchExpensiveQueries changes the isolated test cluster setting
// and verifies the acknowledged effective value. It does not mutate Compiler
// declarations or perform mapping discovery.
func SetElasticsearchExpensiveQueries(
	ctx context.Context,
	client *goelasticsearch.TypedClient,
	config ElasticsearchConfig,
	allowed bool,
) error {
	body, err := json.Marshal(struct {
		Persistent map[string]bool `json:"persistent"`
	}{Persistent: map[string]bool{"search.allow_expensive_queries": allowed}})
	if err != nil {
		return fmt.Errorf("build Elasticsearch cluster setting request")
	}
	if _, err := performElasticsearch(
		ctx, client, config, http.MethodPut, "/_cluster/settings",
		"application/json", body, http.StatusOK,
	); err != nil {
		return fmt.Errorf("set Elasticsearch expensive-query policy")
	}
	settings, err := performElasticsearch(
		ctx, client, config, http.MethodGet,
		"/_cluster/settings?flat_settings=true",
		"", nil, http.StatusOK,
	)
	if err != nil {
		return fmt.Errorf("read Elasticsearch expensive-query policy")
	}
	var response struct {
		Persistent map[string]json.RawMessage `json:"persistent"`
	}
	if json.Unmarshal(settings, &response) != nil {
		return fmt.Errorf("decode Elasticsearch expensive-query policy")
	}
	value, ok := response.Persistent["search.allow_expensive_queries"]
	if !ok || string(value) != strconv.Quote(strconv.FormatBool(allowed)) {
		return fmt.Errorf("Elasticsearch expensive-query policy differs")
	}
	return nil
}

// CloseElasticsearch releases idle HTTP connections held by the typed client.
func CloseElasticsearch(client *goelasticsearch.TypedClient) {
	if client == nil {
		return
	}
	if closer, ok := client.Transport.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func performElasticsearch(
	ctx context.Context,
	client *goelasticsearch.TypedClient,
	config ElasticsearchConfig,
	method string,
	path string,
	contentType string,
	body []byte,
	accepted ...int,
) ([]byte, error) {
	if ctx == nil || client == nil {
		return nil, fmt.Errorf("perform Elasticsearch request: invalid input")
	}
	request, err := http.NewRequestWithContext(
		ctx, method, config.Endpoint()+path, bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("build Elasticsearch request")
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := client.Perform(request)
	if err != nil {
		return nil, fmt.Errorf("perform Elasticsearch request")
	}
	defer response.Body.Close()
	acceptedStatus := false
	for _, status := range accepted {
		acceptedStatus = acceptedStatus || response.StatusCode == status
	}
	if !acceptedStatus {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil, fmt.Errorf("Elasticsearch request returned status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read Elasticsearch response")
	}
	return data, nil
}

func (config ElasticsearchConfig) validate() error {
	if strings.TrimSpace(config.Host) == "" || config.Port == 0 ||
		!validElasticsearchIndex(config.Index) {
		return fmt.Errorf("Elasticsearch configuration is invalid")
	}
	return nil
}

func validElasticsearchIndex(index string) bool {
	if index == "" || strings.TrimSpace(index) != index || len(index) > 255 {
		return false
	}
	for position, character := range index {
		if unicode.IsUpper(character) || unicode.IsControl(character) ||
			strings.ContainsRune(`\\/*?"<>| ,#:`, character) {
			return false
		}
		if position == 0 && strings.ContainsRune(`_-+`, character) {
			return false
		}
	}
	return index != "." && index != ".."
}
