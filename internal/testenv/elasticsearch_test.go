package testenv

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	goelasticsearch "github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
)

type elasticsearchRoundTripFunc func(*http.Request) (*http.Response, error)

func (function elasticsearchRoundTripFunc) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	return function(request)
}

func TestLoadElasticsearchConfig(t *testing.T) {
	config, err := LoadElasticsearchConfig()
	if err != nil {
		t.Fatalf("LoadElasticsearchConfig: %v", err)
	}
	if config.Endpoint() != "http://127.0.0.1:39200" ||
		config.Index != "weave-semantic-records" {
		t.Fatalf("unexpected config: %#v", config)
	}
}

func TestElasticsearchConfigRejectsUnsafeIndex(t *testing.T) {
	for _, index := range []string{"", "Upper", "../index", "index name", "_hidden"} {
		config := ElasticsearchConfig{Host: "127.0.0.1", Port: 9200, Index: index}
		if err := config.validate(); err == nil {
			t.Fatalf("index %q was accepted", index)
		}
	}
}

func TestElasticsearchRequestErrorsAreRedacted(t *testing.T) {
	const (
		responseSecret = "ELASTICSEARCH-RESPONSE-SECRET"
		endpointSecret = "elasticsearch-endpoint-secret.invalid"
	)
	config := ElasticsearchConfig{
		Host:  endpointSecret,
		Port:  9200,
		Index: "weave-semantic-records",
	}
	client, err := goelasticsearch.NewTypedClient(goelasticsearch.Config{
		Transport: elasticsearchRoundTripFunc(func(
			request *http.Request,
		) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header: http.Header{
					"Content-Type":      []string{"application/json"},
					"X-Elastic-Product": []string{"Elasticsearch"},
				},
				Body: io.NopCloser(strings.NewReader(
					`{"error":"` + responseSecret + `"}`,
				)),
				Request: request,
			}, nil
		}),
	})
	if err != nil {
		t.Fatal("open test Elasticsearch client")
	}
	defer CloseElasticsearch(client)

	_, err = performElasticsearch(
		context.Background(), client, config, http.MethodGet,
		"/_redacted", "", nil, http.StatusOK,
	)
	assertElasticsearchErrorOmits(
		t, err, responseSecret, endpointSecret, "_redacted",
	)

	const (
		fieldSecret = "elasticsearch_field_secret"
		valueSecret = "ELASTICSEARCH-VALUE-SECRET"
	)
	query := &types.Query{Term: map[string]types.TermQuery{
		fieldSecret: {Value: valueSecret},
	}}
	_, err = QueryElasticsearchIDs(
		context.Background(), client, config, query,
	)
	assertElasticsearchErrorOmits(
		t, err, responseSecret, endpointSecret, fieldSecret, valueSecret,
	)
}

func assertElasticsearchErrorOmits(
	t *testing.T,
	err error,
	secrets ...string,
) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a redacted Elasticsearch error")
	}
	for _, secret := range secrets {
		if strings.Contains(err.Error(), secret) {
			t.Fatal("Elasticsearch error disclosed request or response data")
		}
	}
}
