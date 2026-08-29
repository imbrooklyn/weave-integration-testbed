//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/imbrooklyn/weave-integration-testbed/internal/demoharness"
	"github.com/imbrooklyn/weave-integration-testbed/internal/fixture"
	"github.com/imbrooklyn/weave/compilertest"
)

func TestElasticsearchCompilerContractAgainstRealServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	environment, cleanup, err := demoharness.NewElasticsearchHarness(ctx, "..")
	if err != nil {
		t.Fatalf("create Elasticsearch harness: %v", err)
	}
	defer cleanup()
	if environment.Server.Version != "9.5.2" ||
		environment.Server.LuceneVersion != "10.5.1" {
		t.Fatalf("unexpected server identity: %#v", environment.Server)
	}
	compilertest.Run(t, environment.Contract)
	results, err := demoharness.RunElasticsearchSeamCases(ctx, environment)
	if err != nil {
		t.Fatalf("Elasticsearch seam cases: %v", err)
	}
	if len(results) != 16 {
		t.Fatalf("Elasticsearch seam result count = %d, want 16", len(results))
	}
	ids, err := environment.Contract.Execute(
		environment.Contract.NativeCondition(fixture.StableIDs()),
	)
	if err != nil {
		t.Fatalf("execute stable-ID Native query: %v", err)
	}
	if err := fixture.CompareIDs(ids, fixture.StableIDs()); err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"real Elasticsearch=%s Lucene=%s canonical=%d seams=%d stable_ids=%v",
		environment.Server.Version,
		environment.Server.LuceneVersion,
		len(compilertest.Scenarios(environment.Contract)),
		len(results),
		ids,
	)
}
