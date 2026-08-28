//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/imbrooklyn/weave"
	weavemongo "github.com/imbrooklyn/weave-adapters/mongo"
	"github.com/imbrooklyn/weave-integration-testbed/internal/demoharness"
	"github.com/imbrooklyn/weave-integration-testbed/internal/fixture"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
	"github.com/imbrooklyn/weave/compilertest"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMongoCompilerContractAgainstRealServer(t *testing.T) {
	runtime := resetMongoFixture(t)
	harness, server, cleanup, err := demoharness.NewMongoHarness(runtime.ctx)
	if err != nil {
		t.Fatalf("create MongoDB harness: %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Errorf("close MongoDB harness: %v", err)
		}
	}()
	if server.Version != runtime.server.Version {
		t.Fatalf("MongoDB server versions differ: %q != %q", server.Version, runtime.server.Version)
	}
	t.Logf("real MongoDB compiler contract server version: %s", server.Version)
	compilertest.Run(t, harness)
}

func TestMongoNativeNegativeOperatorsCannotLeakMissing(t *testing.T) {
	runtime := resetMongoFixture(t)
	collection := runtime.database.Collection(fixture.MongoCollection)
	factory, fields := integrationMongoFactory(t)

	rawNE := bson.D{{
		Key: "nullable_number",
		Value: bson.D{{
			Key:   "$ne",
			Value: int64(2),
		}},
	}}
	rawNEIDs := queryMongoIDs(t, runtime.ctx, collection, rawNE)
	assertIDs(t, rawNEIDs, []string{"r01", "r03", "r04", "r05"})
	guardedNE, err := factory.New().NEQ(fields.nullableNumber, int64(2)).Build()
	if err != nil {
		t.Fatal(err)
	}
	guardedNEIDs := queryMongoIDs(t, runtime.ctx, collection, guardedNE)
	assertIDs(t, guardedNEIDs, []string{"r01", "r05"})

	rawNIN := bson.D{{
		Key: "nullable_number",
		Value: bson.D{{
			Key:   "$nin",
			Value: bson.A{int64(2), int64(5)},
		}},
	}}
	rawNINIDs := queryMongoIDs(t, runtime.ctx, collection, rawNIN)
	assertIDs(t, rawNINIDs, []string{"r01", "r03", "r04"})
	guardedNIN, err := factory.New().
		NotIn(fields.nullableNumber, []int64{2, 5}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	guardedNINIDs := queryMongoIDs(t, runtime.ctx, collection, guardedNIN)
	assertIDs(t, guardedNINIDs, []string{"r01"})

	noneOfEQ, err := factory.New().NoneOf(func(group *weavemongo.Group) {
		group.EQ(fields.nullableNumber, int64(2))
	}).Build()
	if err != nil {
		t.Fatal(err)
	}
	noneOfIDs := queryMongoIDs(t, runtime.ctx, collection, noneOfEQ)
	assertIDs(t, noneOfIDs, []string{"r01", "r03", "r04", "r05"})

	t.Logf(
		"MongoDB %s native $ne IDs=%v, guarded NEQ IDs=%v",
		runtime.server.Version,
		rawNEIDs,
		guardedNEIDs,
	)
	t.Logf(
		"MongoDB %s native $nin IDs=%v, guarded NotIn IDs=%v",
		runtime.server.Version,
		rawNINIDs,
		guardedNINIDs,
	)
}

func TestMongoLiteralTextAndAbsolutePCREAnchors(t *testing.T) {
	runtime := resetMongoFixture(t)
	collection := runtime.database.Collection(fixture.MongoRegexProbeCollection)
	factory, fields := integrationMongoFactory(t)

	tests := []struct {
		name    string
		build   func(*weave.Builder[weavemongo.Filter, weavemongo.Expression])
		wantIDs []string
		pattern string
	}{
		{
			name: "contains metacharacters backslash unicode and newline",
			build: func(builder *weave.Builder[weavemongo.Filter, weavemongo.Expression]) {
				builder.Contains(fields.text, "literal.*\\世界\n")
			},
			wantIDs: []string{"p03", "p04", "p05"},
			pattern: regexp.QuoteMeta("literal.*\\世界\n"),
		},
		{
			name: "absolute prefix",
			build: func(builder *weave.Builder[weavemongo.Filter, weavemongo.Expression]) {
				builder.HasPrefix(fields.text, "literal.*\\世界\n")
			},
			wantIDs: []string{"p03", "p05"},
			pattern: `\A` + regexp.QuoteMeta("literal.*\\世界\n"),
		},
		{
			name: "absolute suffix across newline",
			build: func(builder *weave.Builder[weavemongo.Filter, weavemongo.Expression]) {
				builder.HasSuffix(fields.text, "世界\nend")
			},
			wantIDs: []string{"p03", "p04"},
			pattern: regexp.QuoteMeta("世界\nend") + `\z`,
		},
		{
			name: "absolute suffix excludes final newline",
			build: func(builder *weave.Builder[weavemongo.Filter, weavemongo.Expression]) {
				builder.HasSuffix(fields.text, "alpha")
			},
			wantIDs: []string{"p02"},
			pattern: `alpha\z`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder := factory.New()
			test.build(builder)
			filter, err := builder.Build()
			if err != nil {
				t.Fatal(err)
			}
			if err := demoharness.InspectMongoCondition(test.name, filter); err != nil {
				t.Fatal(err)
			}
			patterns := mongoRegexPatterns(filter)
			if len(patterns) != 1 || patterns[0] != test.pattern {
				t.Fatalf("emitted regex pattern count/value is not the fixed literal form")
			}
			ids := queryMongoIDs(t, runtime.ctx, collection, filter)
			assertIDs(t, ids, test.wantIDs)
			t.Logf("MongoDB %s %s IDs=%v", runtime.server.Version, test.name, ids)
		})
	}

	rawDollar := bson.D{{
		Key: "text_value",
		Value: bson.D{{
			Key:   "$regex",
			Value: "alpha$",
		}},
	}}
	rawDollarIDs := queryMongoIDs(t, runtime.ctx, collection, rawDollar)
	assertIDs(t, rawDollarIDs, []string{"p01", "p02"})
	t.Logf(
		"MongoDB %s raw alpha$ IDs=%v; Adapter alpha\\z IDs=[p02]",
		runtime.server.Version,
		rawDollarIDs,
	)
}

func TestMongoTypedPathValueAndEscapeHatchSecurityBoundaries(t *testing.T) {
	runtime := resetMongoFixture(t)
	mainCollection := runtime.database.Collection(fixture.MongoCollection)
	probeCollection := runtime.database.Collection(fixture.MongoRegexProbeCollection)
	factory, fields := integrationMongoFactory(t)

	for _, path := range []string{
		"$where",
		"profile.$expr",
		"items.$[entry]",
		"field:{$ne:null}",
		"a..b",
		"a b",
	} {
		field, err := weavemongo.NewField[string](path)
		if err == nil || field.Path() != "" {
			t.Fatalf("NewField(injection-like path) = (%#v, %v)", field, err)
		}
	}

	const fieldSecret = "private-$where-field"
	const valueSecret = "private-query-value"
	invalidField, err := factory.New().EQ(
		bson.D{{Key: "$where", Value: fieldSecret}},
		valueSecret,
	).Build()
	assertRedactedZeroCompileError(
		t,
		invalidField,
		err,
		weave.ErrInvalidField,
		fieldSecret,
		valueSecret,
	)
	invalidValue, err := factory.New().EQ(fields.number, valueSecret).Build()
	assertRedactedZeroCompileError(
		t,
		invalidValue,
		err,
		weave.ErrInvalidValue,
		valueSecret,
	)

	for _, build := range []func(*weave.Builder[weavemongo.Filter, weavemongo.Expression]){
		func(builder *weave.Builder[weavemongo.Filter, weavemongo.Expression]) {
			builder.Native(nil)
		},
		func(builder *weave.Builder[weavemongo.Filter, weavemongo.Expression]) {
			builder.Expr(nil)
		},
	} {
		builder := factory.New()
		build(builder)
		filter, err := builder.Build()
		assertRedactedZeroCompileError(t, filter, err, weave.ErrInvalidValue)
	}

	native := bson.D{{
		Key: "_id",
		Value: bson.D{{
			Key:   "$in",
			Value: bson.A{"r02", "r04"},
		}},
	}}
	validNative, err := factory.New().
		Native(native).
		GTE(fields.number, int64(3)).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	assertIDs(
		t,
		queryMongoIDs(t, runtime.ctx, mainCollection, validNative),
		[]string{"r04"},
	)
	validExpr, err := factory.New().AnyOf(func(group *weavemongo.Group) {
		group.Expr(bson.D{{
			Key: "_id",
			Value: bson.D{{
				Key:   "$in",
				Value: bson.A{"r01", "r03"},
			}},
		}}).EQ(fields.number, int64(6))
	}).Build()
	if err != nil {
		t.Fatal(err)
	}
	assertIDs(
		t,
		queryMongoIDs(t, runtime.ctx, mainCollection, validExpr),
		[]string{"r01", "r03", "r06"},
	)

	injectionFilter, err := factory.New().
		Contains(fields.text, fixture.MongoInjectionText).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := demoharness.InspectMongoCondition("injection-like BSON value", injectionFilter); err != nil {
		t.Fatal(err)
	}
	if mongoContainsKey(injectionFilter, "$where") {
		t.Fatal("injection-like value became a BSON operator key")
	}
	patterns := mongoRegexPatterns(injectionFilter)
	if len(patterns) != 1 || patterns[0] != regexp.QuoteMeta(fixture.MongoInjectionText) {
		t.Fatal("injection-like value did not remain one quoted BSON string value")
	}
	assertIDs(
		t,
		queryMongoIDs(t, runtime.ctx, probeCollection, injectionFilter),
		[]string{"p06"},
	)
}

func TestMongoCompileBSONOrderDeterminismAndConcurrency(t *testing.T) {
	runtime := resetMongoFixture(t)
	collection := runtime.database.Collection(fixture.MongoCollection)
	factory, fields := integrationMongoFactory(t)
	predicate, err := factory.New().
		GTE(fields.number, int64(2)).
		AnyOf(func(group *weavemongo.Group) {
			group.Contains(fields.text, "prefix").
				In(fields.number, []int64{2, 6})
		}).
		Predicate()
	if err != nil {
		t.Fatal(err)
	}
	want, err := factory.Compile(predicate)
	if err != nil {
		t.Fatal(err)
	}
	if err := demoharness.InspectMongoCondition("determinism", want); err != nil {
		t.Fatal(err)
	}
	wantBytes, err := bson.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{"r02", "r03", "r04", "r06"}
	assertIDs(t, queryMongoIDs(t, runtime.ctx, collection, want), wantIDs)

	for iteration := range 100 {
		filter, err := factory.Compile(predicate)
		if err != nil {
			t.Fatalf("Compile(%d) error = %v", iteration, err)
		}
		encoded, err := bson.Marshal(filter)
		if err != nil || !bytes.Equal(encoded, wantBytes) {
			t.Fatalf("Compile(%d) changed ordered BSON bytes", iteration)
		}
	}

	mutated, err := factory.Compile(predicate)
	if err != nil {
		t.Fatal(err)
	}
	mutated[0].Key = "$caller_mutation"
	again, err := factory.Compile(predicate)
	if err != nil {
		t.Fatal(err)
	}
	againBytes, err := bson.Marshal(again)
	if err != nil || !bytes.Equal(againBytes, wantBytes) {
		t.Fatal("Compile reused caller-visible top-level BSON storage")
	}

	const workers = 32
	errorsFound := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			filter, err := factory.Compile(predicate)
			if err != nil {
				errorsFound <- err
				return
			}
			encoded, err := bson.Marshal(filter)
			if err != nil {
				errorsFound <- err
				return
			}
			if !bytes.Equal(encoded, wantBytes) {
				errorsFound <- errors.New("concurrent Compile changed BSON order")
				return
			}
			ids, err := testenv.QueryMongoIDs(runtime.ctx, collection, filter)
			if err != nil {
				errorsFound <- err
				return
			}
			if err := fixture.CompareIDs(ids, wantIDs); err != nil {
				errorsFound <- err
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent Compile/Find error = %v", err)
	}
	t.Logf("MongoDB %s produced identical bson.D/bson.A bytes and IDs in %d workers", runtime.server.Version, workers)
}

type integrationMongoFields struct {
	number         weavemongo.Field[int64]
	text           weavemongo.Field[string]
	nullableNumber weavemongo.Field[int64]
}

func integrationMongoFactory(
	t *testing.T,
) (*weavemongo.Factory, integrationMongoFields) {
	t.Helper()
	factory, err := weavemongo.NewFactory(weavemongo.MongoDB60Plus)
	if err != nil {
		t.Fatal(err)
	}
	number, err := weavemongo.NewField[int64]("number_value")
	if err != nil {
		t.Fatal(err)
	}
	text, err := weavemongo.NewField[string]("text_value")
	if err != nil {
		t.Fatal(err)
	}
	nullableNumber, err := weavemongo.NewField[int64]("nullable_number")
	if err != nil {
		t.Fatal(err)
	}
	return factory, integrationMongoFields{
		number:         number,
		text:           text,
		nullableNumber: nullableNumber,
	}
}

type mongoRuntime struct {
	ctx      context.Context
	database *mongodriver.Database
	server   testenv.MongoServerInfo
}

func resetMongoFixture(t *testing.T) mongoRuntime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backendTimeout)
	config, err := testenv.LoadMongoConfig()
	if err != nil {
		cancel()
		t.Fatalf("load MongoDB configuration: %v", err)
	}
	client, err := testenv.OpenMongo(config)
	if err != nil {
		cancel()
		t.Fatalf("open MongoDB fixture client: %v", err)
	}
	t.Cleanup(func() {
		if err := testenv.CloseMongo(client); err != nil {
			t.Errorf("close MongoDB fixture client: %v", err)
		}
		cancel()
	})
	if err := testenv.WaitForMongo(ctx, client, 250*time.Millisecond); err != nil {
		t.Fatalf("wait for MongoDB fixture: %v", err)
	}
	server, err := testenv.ReadMongoServerInfo(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	database := client.Database(config.Database)
	if err := testenv.ResetMongo(ctx, database); err != nil {
		t.Fatalf("reset MongoDB fixture: %v", err)
	}
	t.Logf("real MongoDB fixture reset on server %s", server.Version)
	return mongoRuntime{ctx: ctx, database: database, server: server}
}

func queryMongoIDs(
	t *testing.T,
	ctx context.Context,
	collection *mongodriver.Collection,
	filter bson.D,
) []string {
	t.Helper()
	ids, err := testenv.QueryMongoIDs(ctx, collection, filter)
	if err != nil {
		t.Fatalf("query MongoDB IDs: %v", err)
	}
	return ids
}

func assertIDs(t *testing.T, got, want []string) {
	t.Helper()
	if err := fixture.CompareIDs(got, want); err != nil {
		t.Fatal(err)
	}
}

func assertRedactedZeroCompileError(
	t *testing.T,
	filter bson.D,
	err error,
	want error,
	secrets ...string,
) {
	t.Helper()
	if filter != nil {
		t.Fatalf("Compile failure returned a nonzero BSON document")
	}
	if !errors.Is(err, weave.ErrCompile) || !errors.Is(err, want) {
		t.Fatalf("Compile error = %v, want ErrCompile and %v", err, want)
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(err.Error(), secret) {
			t.Fatalf("Compile error disclosed a query value")
		}
	}
}

func mongoContainsKey(value any, key string) bool {
	switch typed := value.(type) {
	case bson.D:
		for _, element := range typed {
			if element.Key == key || mongoContainsKey(element.Value, key) {
				return true
			}
		}
	case bson.A:
		for _, element := range typed {
			if mongoContainsKey(element, key) {
				return true
			}
		}
	}
	return false
}

func mongoRegexPatterns(value any) []string {
	var patterns []string
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case bson.D:
			for _, element := range typed {
				if element.Key == "$regex" {
					if pattern, ok := element.Value.(string); ok {
						patterns = append(patterns, pattern)
					}
				}
				visit(element.Value)
			}
		case bson.A:
			for _, element := range typed {
				visit(element)
			}
		}
	}
	visit(value)
	return patterns
}
