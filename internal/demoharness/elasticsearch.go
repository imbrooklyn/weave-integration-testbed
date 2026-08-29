package demoharness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	goelasticsearch "github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/imbrooklyn/weave"
	weavees "github.com/imbrooklyn/weave-adapters/elasticsearch"
	"github.com/imbrooklyn/weave-integration-testbed/internal/fixture"
	"github.com/imbrooklyn/weave-integration-testbed/internal/scenario"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
	"github.com/imbrooklyn/weave/compilertest"
)

const nullNumberSentinel = int64(-9223372036854775808)

// ElasticsearchFields contains the canonical compilertest bindings and the
// search-only field seams exercised by both the Demo and integration tests.
type ElasticsearchFields struct {
	Canonical         compilertest.Fields
	Decimal           weavees.Field[float64]
	Date              weavees.Field[time.Time]
	Boolean           weavees.Field[bool]
	Analyzed          weavees.Field[string]
	MultiValued       weavees.Field[string]
	Nested            weavees.Field[string]
	NormalizedKeyword weavees.Field[string]
	ExpensiveKeyword  weavees.Field[string]
	PatternWildcard   weavees.Field[string]
	RawNullKeyword    weavees.Field[string]
	UntrackedKeyword  weavees.Field[string]
	EmptyKeyword      weavees.Field[string]
}

// ElasticsearchHarness owns real-service wiring around immutable Adapter
// declarations. The Compiler itself owns none of these request resources.
type ElasticsearchHarness struct {
	Contract          compilertest.Harness[weavees.Query, weavees.Expression]
	Fields            ElasticsearchFields
	StrictCompiler    weavees.Compiler
	ExpensiveCompiler weavees.Compiler
	StrictFactory     *weavees.Factory
	ExpensiveFactory  *weavees.Factory
	TypedClient       *goelasticsearch.TypedClient
	Config            testenv.ElasticsearchConfig
	Server            testenv.ElasticsearchServerInfo
}

// RunElasticsearch recreates the fixed fixture, runs all applicable canonical
// scenarios, and then executes the shared Elasticsearch-specific seam cases.
func RunElasticsearch(ctx context.Context) (scenario.Report, error) {
	environment, cleanup, err := NewElasticsearchHarness(ctx, ".")
	if err != nil {
		return scenario.Report{}, err
	}
	defer cleanup()
	report, err := scenario.Run(
		ctx,
		"elasticsearch",
		"elasticsearch-"+environment.Server.Version+
			"-lucene-"+environment.Server.LuceneVersion,
		environment.Contract,
	)
	if err != nil {
		return report, err
	}
	results, err := RunElasticsearchSeamCases(ctx, environment)
	if err != nil {
		return report, err
	}
	report.Results = append(report.Results, results...)
	return report, nil
}

// NewElasticsearchHarness opens and verifies the exact real server, recreates
// the canonical fixture, and returns shared strict/expensive Compiler wiring.
func NewElasticsearchHarness(
	ctx context.Context,
	root string,
) (*ElasticsearchHarness, func(), error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("create Elasticsearch harness: nil context")
	}
	fields, mapping, err := newElasticsearchFields()
	if err != nil {
		return nil, nil, err
	}
	strictCompiler, err := weavees.NewCompiler(
		weavees.Elasticsearch95NoExpensiveQueries,
		mapping,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create strict Elasticsearch Compiler: %w", err)
	}
	expensiveCompiler, err := weavees.NewCompiler(
		weavees.Elasticsearch95ExpensiveQueries,
		mapping,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create expensive Elasticsearch Compiler: %w", err)
	}
	config, err := testenv.LoadElasticsearchConfig()
	if err != nil {
		return nil, nil, err
	}
	client, err := testenv.OpenElasticsearch(config)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = testenv.SetElasticsearchExpensiveQueries(
			cleanupCtx, client, config, true,
		)
		testenv.CloseElasticsearch(client)
	}
	if err := testenv.WaitForElasticsearch(ctx, client, 250*time.Millisecond); err != nil {
		cleanup()
		return nil, nil, err
	}
	server, err := testenv.ReadElasticsearchServerInfo(ctx, client)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := testenv.ResetElasticsearch(ctx, client, config, root); err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := testenv.SetElasticsearchExpensiveQueries(
		ctx, client, config, false,
	); err != nil {
		cleanup()
		return nil, nil, err
	}

	strictFactory := weave.NewFactory[weavees.Query, weavees.Expression](strictCompiler)
	expensiveFactory := weave.NewFactory[weavees.Query, weavees.Expression](expensiveCompiler)
	environment := &ElasticsearchHarness{
		Fields:            fields,
		StrictCompiler:    strictCompiler,
		ExpensiveCompiler: expensiveCompiler,
		StrictFactory:     strictFactory,
		ExpensiveFactory:  expensiveFactory,
		TypedClient:       client,
		Config:            config,
		Server:            server,
	}
	environment.Contract = compilertest.Harness[weavees.Query, weavees.Expression]{
		Factory:  strictFactory,
		Fields:   fields.Canonical,
		Resolver: strictCompiler,
		Execute: func(query weavees.Query) ([]string, error) {
			return testenv.QueryElasticsearchIDs(ctx, client, config, query)
		},
		InspectCondition: InspectElasticsearchQuery,
		NativeCondition: func(ids []string) weavees.Query {
			return &types.Query{Ids: &types.IdsQuery{Values: append([]string(nil), ids...)}}
		},
		NativeExpression: func(ids []string) weavees.Expression {
			return esdsl.NewIdsQuery().Values(ids...)
		},
		NilLikeNativeCondition: func() weavees.Query { return nil },
		NilLikeNativeExpression: func() weavees.Expression {
			var query *types.Query
			return query
		},
		DistinguishesMissing: true,
	}
	return environment, cleanup, nil
}

// RunElasticsearchSeamCases executes the non-canonical mapping and policy
// seams shared by the Demo and integration suite.
func RunElasticsearchSeamCases(
	ctx context.Context,
	environment *ElasticsearchHarness,
) ([]scenario.Result, error) {
	if ctx == nil || environment == nil || environment.TypedClient == nil {
		return nil, fmt.Errorf("Elasticsearch seam cases: invalid harness")
	}
	if err := assertElasticsearchCapabilityBoundaries(environment); err != nil {
		return nil, err
	}
	if err := assertElasticsearchRedactedStableError(environment); err != nil {
		return nil, err
	}
	if err := assertElasticsearchServerExpensiveBoundary(ctx, environment); err != nil {
		return nil, err
	}

	strictCases := []elasticsearchCase{
		{
			name: "search decimal between",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.Between(environment.Fields.Decimal, 2.5, 4.5)
			},
			want: []string{"r02", "r03", "r04"},
		},
		{
			name: "search date range",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.GTE(
					environment.Fields.Date,
					time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
				).LTE(
					environment.Fields.Date,
					time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
				)
			},
			want: []string{"r02", "r03", "r04"},
		},
		{
			name: "search boolean equality",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.EQ(environment.Fields.Boolean, true)
			},
			want: []string{"r01", "r03", "r05"},
		},
		{
			name: "search analyzed text through Expr",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.Expr(esdsl.NewMatchQuery("analyzed_text", "quick"))
			},
			want: []string{"r01", "r02"},
		},
		{
			name: "search literal wildcard metacharacters",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.Contains(environment.Fields.PatternWildcard, `*?\ 世界`)
			},
			want: []string{"r02"},
		},
		{
			name: "search literal prefix metacharacters",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.HasPrefix(environment.Fields.PatternWildcard, `a*b?c\`)
			},
			want: []string{"r04"},
		},
		{
			name: "search Unicode suffix",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.HasSuffix(environment.Fields.PatternWildcard, "世界-end")
			},
			want: []string{"r05"},
		},
		{
			name: "search null_value explicit null",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.IsNull(environment.Fields.RawNullKeyword)
			},
			want: []string{"r03"},
		},
		{
			name: "search null_value non-null guard",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.NotNull(environment.Fields.RawNullKeyword)
			},
			want: []string{"r01", "r02", "r05", "r06"},
		},
		{
			name: "search source null versus indexed existence",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.Expr(esdsl.NewExistsQuery().Field("source_null"))
			},
			want: []string{"r01", "r02", "r05", "r06"},
		},
		{
			name: "search empty array indexed absence",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.Expr(esdsl.NewExistsQuery().Field("empty_array_keyword"))
			},
			want: []string{"r01", "r02", "r03", "r05", "r06"},
		},
		{
			name: "search empty string equality",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.EQ(environment.Fields.EmptyKeyword, "")
			},
			want: []string{"r01"},
		},
		{
			name: "search lowercase normalizer equality",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.EQ(environment.Fields.NormalizedKeyword, "alpha")
			},
			want: []string{"r01"},
		},
		{
			name: "search depth eight combination",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.AllOf(func(group *weavees.Group) {
					addDeepElasticsearchGroup(group, environment.Fields.Canonical.Number, 8)
				})
			},
			want: []string{"r02"},
		},
	}
	results, err := executeElasticsearchCases(
		ctx, environment, environment.StrictFactory, strictCases,
	)
	if err != nil {
		return nil, err
	}

	if err := testenv.SetElasticsearchExpensiveQueries(
		ctx, environment.TypedClient, environment.Config, true,
	); err != nil {
		return nil, err
	}
	expensiveCases := []elasticsearchCase{
		{
			name: "search expensive keyword literal wildcard",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.Contains(environment.Fields.ExpensiveKeyword, `*?\ 世界`)
			},
			want: []string{"r02"},
		},
		{
			name: "search expensive normalized prefix",
			build: func(builder *weave.Builder[weavees.Query, weavees.Expression]) {
				builder.HasPrefix(environment.Fields.NormalizedKeyword, "al")
			},
			want: []string{"r01"},
		},
	}
	expensiveResults, err := executeElasticsearchCases(
		ctx, environment, environment.ExpensiveFactory, expensiveCases,
	)
	restoreErr := testenv.SetElasticsearchExpensiveQueries(
		ctx, environment.TypedClient, environment.Config, false,
	)
	if err != nil {
		return nil, err
	}
	if restoreErr != nil {
		return nil, restoreErr
	}
	return append(results, expensiveResults...), nil
}

type elasticsearchCase struct {
	name  string
	build func(*weave.Builder[weavees.Query, weavees.Expression])
	want  []string
}

func executeElasticsearchCases(
	ctx context.Context,
	environment *ElasticsearchHarness,
	factory *weavees.Factory,
	cases []elasticsearchCase,
) ([]scenario.Result, error) {
	results := make([]scenario.Result, 0, len(cases))
	for _, test := range cases {
		builder := factory.New()
		test.build(builder)
		query, err := builder.Build()
		if err != nil {
			return nil, fmt.Errorf("Elasticsearch seam case %q build: %w", test.name, err)
		}
		if err := InspectElasticsearchQuery(test.name, query); err != nil {
			return nil, fmt.Errorf("Elasticsearch seam case %q inspect: %w", test.name, err)
		}
		ids, err := testenv.QueryElasticsearchIDs(
			ctx, environment.TypedClient, environment.Config, query,
		)
		if err != nil {
			return nil, fmt.Errorf("Elasticsearch seam case %q execute: %w", test.name, err)
		}
		if err := fixture.CompareIDs(ids, test.want); err != nil {
			return nil, fmt.Errorf("Elasticsearch seam case %q: %w", test.name, err)
		}
		canonical, err := fixture.CanonicalIDs(ids)
		if err != nil {
			return nil, err
		}
		results = append(results, scenario.Result{Name: test.name, IDs: canonical})
	}
	return results, nil
}

func assertElasticsearchCapabilityBoundaries(
	environment *ElasticsearchHarness,
) error {
	for name, field := range map[string]any{
		"analyzed":     environment.Fields.Analyzed,
		"multi-valued": environment.Fields.MultiValued,
		"nested":       environment.Fields.Nested,
	} {
		capabilities, err := environment.StrictCompiler.CapabilitiesFor(field)
		if err != nil || capabilities.Operators.Count() != 0 {
			return fmt.Errorf("Elasticsearch %s field capability boundary differs", name)
		}
	}
	untracked, err := environment.StrictCompiler.CapabilitiesFor(
		environment.Fields.UntrackedKeyword,
	)
	if err != nil || untracked.Operators.Has(weave.OperatorIsNull) ||
		untracked.Operators.Has(weave.OperatorNotNull) {
		return fmt.Errorf("Elasticsearch untracked-null capability boundary differs")
	}
	strictExpensive, err := environment.StrictCompiler.CapabilitiesFor(
		environment.Fields.ExpensiveKeyword,
	)
	if err != nil {
		return err
	}
	for _, operator := range []weave.Operator{
		weave.OperatorContains,
		weave.OperatorHasPrefix,
		weave.OperatorHasSuffix,
	} {
		if strictExpensive.Operators.Has(operator) {
			return fmt.Errorf("strict Elasticsearch keyword exposes an expensive operator")
		}
	}
	builder := environment.StrictFactory.New()
	builder.Contains(environment.Fields.ExpensiveKeyword, "secret-pattern")
	query, err := builder.Build()
	if query != nil || !errors.Is(err, weave.ErrOperatorNotApplicable) {
		return fmt.Errorf("strict Elasticsearch keyword did not fail before execution")
	}
	return nil
}

func assertElasticsearchRedactedStableError(
	environment *ElasticsearchHarness,
) error {
	builder := environment.StrictFactory.New()
	builder.EQ(environment.Fields.Canonical.Number, "first-secret").
		EQ("second-secret-field", int64(2))
	query, err := builder.Build()
	if query != nil || !errors.Is(err, weave.ErrInvalidValue) {
		return fmt.Errorf("Elasticsearch stable validation error contract differs")
	}
	var structured *weave.Error
	if !errors.As(err, &structured) || structured.Origin.Sequence != 1 ||
		structured.Phase != weave.PhaseValidate {
		return fmt.Errorf("Elasticsearch stable first error metadata differs")
	}
	if strings.Contains(err.Error(), "first-secret") ||
		strings.Contains(err.Error(), "second-secret-field") {
		return fmt.Errorf("Elasticsearch validation error leaked query data")
	}
	return nil
}

func assertElasticsearchServerExpensiveBoundary(
	ctx context.Context,
	environment *ElasticsearchHarness,
) error {
	builder := environment.StrictFactory.New()
	builder.Expr(esdsl.NewWildcardQuery("expensive_keyword", "*plain*"))
	query, err := builder.Build()
	if err != nil || query == nil {
		return fmt.Errorf("build Elasticsearch expensive Expr boundary")
	}
	if _, err := testenv.QueryElasticsearchIDs(
		ctx,
		environment.TypedClient,
		environment.Config,
		query,
	); err == nil {
		return fmt.Errorf("Elasticsearch accepted an expensive keyword query while disabled")
	}
	return nil
}

func addDeepElasticsearchGroup(group *weavees.Group, field any, depth int) {
	if depth == 0 {
		group.EQ(field, int64(2))
		return
	}
	group.AllOf(func(child *weavees.Group) {
		addDeepElasticsearchGroup(child, field, depth-1)
	})
}

// InspectElasticsearchQuery verifies that a compiled value is a non-empty,
// single-variant typed Query without exposing its field names or values.
func InspectElasticsearchQuery(_ string, query weavees.Query) error {
	if query == nil {
		return fmt.Errorf("Elasticsearch query is nil")
	}
	encoded, err := json.Marshal(query)
	if err != nil {
		return fmt.Errorf("Elasticsearch query is not JSON encodable")
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(encoded, &top) != nil || len(top) != 1 {
		return fmt.Errorf("Elasticsearch query is not one typed Query variant")
	}
	return nil
}

func newElasticsearchFields() (ElasticsearchFields, weavees.Mapping, error) {
	var fields ElasticsearchFields
	textMarker, err := searchField(weavees.FieldSpec[string]{
		Path: "text_value_state", Type: weavees.MappingKeyword,
		CompleteValueIndex: true,
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	textCompanion, err := weavees.NewCompanionMarker(textMarker, "null", "value")
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	nullableTextMarker, err := searchField(weavees.FieldSpec[string]{
		Path: "nullable_text_state", Type: weavees.MappingKeyword,
		CompleteValueIndex: true,
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	nullableTextCompanion, err := weavees.NewCompanionMarker(
		nullableTextMarker, "null", "value",
	)
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	equalityMarker, err := searchField(weavees.FieldSpec[string]{
		Path: "equality_only_text_state", Type: weavees.MappingKeyword,
		CompleteValueIndex: true,
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	equalityCompanion, err := weavees.NewCompanionMarker(
		equalityMarker, "null", "value",
	)
	if err != nil {
		return fields, weavees.Mapping{}, err
	}

	numericOperators := weave.NewOperatorSet(
		weave.OperatorEQ, weave.OperatorNEQ,
		weave.OperatorLT, weave.OperatorLTE,
		weave.OperatorGT, weave.OperatorGTE,
		weave.OperatorIn, weave.OperatorNotIn,
		weave.OperatorBetween, weave.OperatorIsNull, weave.OperatorNotNull,
	)
	textOperators := weave.NewOperatorSet(
		weave.OperatorEQ, weave.OperatorNEQ,
		weave.OperatorIn, weave.OperatorNotIn,
		weave.OperatorIsNull, weave.OperatorNotNull,
		weave.OperatorContains, weave.OperatorHasPrefix, weave.OperatorHasSuffix,
	)
	equalityOperators := weave.NewOperatorSet(
		weave.OperatorEQ, weave.OperatorNEQ,
		weave.OperatorIn, weave.OperatorNotIn,
		weave.OperatorIsNull, weave.OperatorNotNull,
	)
	number, err := searchField(weavees.FieldSpec[int64]{
		Path: "number_value", Type: weavees.MappingLong,
		CompleteValueIndex: true, Nulls: weavees.IndexNullAs(nullNumberSentinel),
		Operators: numericOperators,
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	text, err := searchField(weavees.FieldSpec[string]{
		Path: "text_value", Type: weavees.MappingWildcard,
		CompleteValueIndex: true, Nulls: weavees.MarkNullWith[string](textCompanion),
		Operators: textOperators,
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	nullableNumber, err := searchField(weavees.FieldSpec[int64]{
		Path: "nullable_number", Type: weavees.MappingLong,
		CompleteValueIndex: true, Nulls: weavees.IndexNullAs(nullNumberSentinel),
		Operators: numericOperators,
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	nullableText, err := searchField(weavees.FieldSpec[string]{
		Path: "nullable_text", Type: weavees.MappingWildcard,
		CompleteValueIndex: true,
		Nulls:              weavees.MarkNullWith[string](nullableTextCompanion),
		Operators:          textOperators,
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	equalityText, err := searchField(weavees.FieldSpec[string]{
		Path: "equality_only_text", Type: weavees.MappingWildcard,
		CompleteValueIndex: true,
		Nulls:              weavees.MarkNullWith[string](equalityCompanion),
		Operators:          equalityOperators,
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	fields.Canonical = compilertest.Fields{
		Number: number, Text: text,
		NullableNumber: nullableNumber, NullableText: nullableText,
		EqualityOnlyText: equalityText,
	}

	fields.Decimal, err = searchField(weavees.FieldSpec[float64]{
		Path: "decimal_value", Type: weavees.MappingDouble,
		CompleteValueIndex: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ, weave.OperatorNEQ,
			weave.OperatorLT, weave.OperatorLTE,
			weave.OperatorGT, weave.OperatorGTE,
			weave.OperatorIn, weave.OperatorNotIn, weave.OperatorBetween,
		),
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	fields.Date, err = searchField(weavees.FieldSpec[time.Time]{
		Path: "created_at", Type: weavees.MappingDate,
		CompleteValueIndex: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ, weave.OperatorNEQ,
			weave.OperatorLT, weave.OperatorLTE,
			weave.OperatorGT, weave.OperatorGTE,
			weave.OperatorIn, weave.OperatorNotIn,
		),
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	fields.Boolean, err = searchField(weavees.FieldSpec[bool]{
		Path: "bool_value", Type: weavees.MappingBoolean,
		CompleteValueIndex: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ, weave.OperatorNEQ,
			weave.OperatorIn, weave.OperatorNotIn,
		),
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	fields.Analyzed, err = searchField(weavees.FieldSpec[string]{
		Path: "analyzed_text", Type: weavees.MappingText,
		CompleteValueIndex: true,
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	fields.MultiValued, err = searchField(weavees.FieldSpec[string]{
		Path: "tags", Type: weavees.MappingKeyword,
		CompleteValueIndex: true, MultiValued: true,
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	fields.Nested, err = searchField(weavees.FieldSpec[string]{
		Path: "nested_items.name", Type: weavees.MappingKeyword,
		CompleteValueIndex: true, Nested: true,
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	fields.NormalizedKeyword, err = searchField(weavees.FieldSpec[string]{
		Path: "normalized_keyword", Type: weavees.MappingKeyword,
		CompleteValueIndex: true, Normalizer: "weave_lowercase",
		AllowExpensiveWildcard: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ, weave.OperatorNEQ,
			weave.OperatorIn, weave.OperatorNotIn,
			weave.OperatorContains, weave.OperatorHasPrefix, weave.OperatorHasSuffix,
		),
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	fields.ExpensiveKeyword, err = searchField(weavees.FieldSpec[string]{
		Path: "expensive_keyword", Type: weavees.MappingKeyword,
		CompleteValueIndex: true, AllowExpensiveWildcard: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ, weave.OperatorNEQ,
			weave.OperatorIn, weave.OperatorNotIn,
			weave.OperatorContains, weave.OperatorHasPrefix, weave.OperatorHasSuffix,
		),
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	fields.PatternWildcard, err = searchField(weavees.FieldSpec[string]{
		Path: "pattern_wildcard", Type: weavees.MappingWildcard,
		CompleteValueIndex: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ, weave.OperatorNEQ,
			weave.OperatorIn, weave.OperatorNotIn,
			weave.OperatorContains, weave.OperatorHasPrefix, weave.OperatorHasSuffix,
		),
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	fields.RawNullKeyword, err = searchField(weavees.FieldSpec[string]{
		Path: "raw_null_keyword", Type: weavees.MappingKeyword,
		CompleteValueIndex: true, Nulls: weavees.IndexNullAs("__WEAVE_NULL__"),
		Operators: equalityOperators,
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	fields.UntrackedKeyword, err = searchField(weavees.FieldSpec[string]{
		Path: "untracked_keyword", Type: weavees.MappingKeyword,
		CompleteValueIndex: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ, weave.OperatorNEQ,
			weave.OperatorIn, weave.OperatorNotIn,
		),
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}
	fields.EmptyKeyword, err = searchField(weavees.FieldSpec[string]{
		Path: "empty_keyword", Type: weavees.MappingKeyword,
		CompleteValueIndex: true,
		Operators: weave.NewOperatorSet(
			weave.OperatorEQ, weave.OperatorNEQ,
			weave.OperatorIn, weave.OperatorNotIn,
		),
	})
	if err != nil {
		return fields, weavees.Mapping{}, err
	}

	mapping, err := weavees.NewMapping(
		textMarker, nullableTextMarker, equalityMarker,
		number, text, nullableNumber, nullableText, equalityText,
		fields.Decimal, fields.Date, fields.Boolean,
		fields.Analyzed, fields.MultiValued, fields.Nested,
		fields.NormalizedKeyword, fields.ExpensiveKeyword,
		fields.PatternWildcard, fields.RawNullKeyword,
		fields.UntrackedKeyword, fields.EmptyKeyword,
	)
	if err != nil {
		return fields, weavees.Mapping{}, fmt.Errorf("create Elasticsearch Mapping: %w", err)
	}
	return fields, mapping, nil
}

func searchField[T any](
	spec weavees.FieldSpec[T],
) (weavees.Field[T], error) {
	field, err := weavees.NewField(spec)
	if err != nil {
		return weavees.Field[T]{}, fmt.Errorf("create Elasticsearch Field: %w", err)
	}
	return field, nil
}
