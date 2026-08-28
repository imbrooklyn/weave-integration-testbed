package demoharness

import (
	"context"
	"fmt"
	"time"

	"github.com/imbrooklyn/weave"
	weavemongo "github.com/imbrooklyn/weave-adapters/mongo"
	"github.com/imbrooklyn/weave-integration-testbed/internal/fixture"
	"github.com/imbrooklyn/weave-integration-testbed/internal/scenario"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
	"github.com/imbrooklyn/weave/compilertest"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// RunMongo executes every canonical scenario against a real MongoDB 6.0+
// service and includes the exact server version in the report identity.
func RunMongo(ctx context.Context) (report scenario.Report, err error) {
	harness, server, cleanup, err := NewMongoHarness(ctx)
	if err != nil {
		return report, err
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil && err == nil {
			err = cleanupErr
		}
	}()
	return scenario.Run(ctx, "mongo", "mongodb-"+server.Version, harness)
}

// NewMongoHarness opens the configured service and returns the canonical
// compiler-test harness, non-secret server identity, and cleanup function.
func NewMongoHarness(
	ctx context.Context,
) (
	compilertest.Harness[weavemongo.Filter, weavemongo.Expression],
	testenv.MongoServerInfo,
	func() error,
	error,
) {
	var zero compilertest.Harness[weavemongo.Filter, weavemongo.Expression]
	if ctx == nil {
		return zero, testenv.MongoServerInfo{}, nil, fmt.Errorf("create MongoDB harness: nil context")
	}
	compiler, err := weavemongo.NewCompiler(weavemongo.MongoDB60Plus)
	if err != nil {
		return zero, testenv.MongoServerInfo{}, nil, fmt.Errorf("create MongoDB Compiler: %w", err)
	}
	fields, err := newMongoFields()
	if err != nil {
		return zero, testenv.MongoServerInfo{}, nil, err
	}
	config, err := testenv.LoadMongoConfig()
	if err != nil {
		return zero, testenv.MongoServerInfo{}, nil, err
	}
	client, err := testenv.OpenMongo(config)
	if err != nil {
		return zero, testenv.MongoServerInfo{}, nil, err
	}
	cleanup := func() error { return testenv.CloseMongo(client) }
	if err := testenv.WaitForMongo(ctx, client, 250*time.Millisecond); err != nil {
		_ = cleanup()
		return zero, testenv.MongoServerInfo{}, nil, err
	}
	server, err := testenv.ReadMongoServerInfo(ctx, client)
	if err != nil {
		_ = cleanup()
		return zero, testenv.MongoServerInfo{}, nil, err
	}
	collection := client.Database(config.Database).Collection(fixture.MongoCollection)
	factory := weave.NewFactory[weavemongo.Filter, weavemongo.Expression](compiler)
	harness := compilertest.Harness[weavemongo.Filter, weavemongo.Expression]{
		Factory:  factory,
		Fields:   fields,
		Resolver: compiler,
		Execute: func(filter weavemongo.Filter) ([]string, error) {
			return testenv.QueryMongoIDs(ctx, collection, filter)
		},
		InspectCondition: InspectMongoCondition,
		NativeCondition: func(ids []string) weavemongo.Filter {
			return mongoIDFilter(ids)
		},
		NativeExpression: func(ids []string) weavemongo.Expression {
			return mongoIDFilter(ids)
		},
		NilLikeNativeCondition:  func() weavemongo.Filter { return nil },
		NilLikeNativeExpression: func() weavemongo.Expression { return nil },
		DistinguishesMissing:    true,
	}
	return harness, server, cleanup, nil
}

func newMongoFields() (compilertest.Fields, error) {
	number, err := weavemongo.NewField[int64]("number_value")
	if err != nil {
		return compilertest.Fields{}, fmt.Errorf("create MongoDB number Field: %w", err)
	}
	text, err := weavemongo.NewField[string]("text_value")
	if err != nil {
		return compilertest.Fields{}, fmt.Errorf("create MongoDB text Field: %w", err)
	}
	nullableNumber, err := weavemongo.NewField[int64]("nullable_number")
	if err != nil {
		return compilertest.Fields{}, fmt.Errorf("create MongoDB nullable-number Field: %w", err)
	}
	nullableText, err := weavemongo.NewField[string]("nullable_text")
	if err != nil {
		return compilertest.Fields{}, fmt.Errorf("create MongoDB nullable-text Field: %w", err)
	}
	equalityOnlyText, err := weavemongo.NewFieldWithOperators[string](
		"equality_only_text",
		equalityOnlyOperators...,
	)
	if err != nil {
		return compilertest.Fields{}, fmt.Errorf("create MongoDB equality-only Field: %w", err)
	}
	return compilertest.Fields{
		Number:           number,
		Text:             text,
		NullableNumber:   nullableNumber,
		NullableText:     nullableText,
		EqualityOnlyText: equalityOnlyText,
	}, nil
}

func mongoIDFilter(ids []string) bson.D {
	values := make(bson.A, len(ids))
	for index := range ids {
		values[index] = ids[index]
	}
	return bson.D{{
		Key: "_id",
		Value: bson.D{{
			Key:   "$in",
			Value: values,
		}},
	}}
}

// InspectMongoCondition verifies ordered BSON topology and driver encodability
// without logging field names, patterns, or stored query values.
func InspectMongoCondition(_ string, filter weavemongo.Filter) error {
	if filter == nil {
		return fmt.Errorf("MongoDB condition is nil")
	}
	if err := inspectOrderedMongoBSON(filter); err != nil {
		return err
	}
	if _, err := bson.Marshal(filter); err != nil {
		return fmt.Errorf("MongoDB condition is not BSON encodable")
	}
	return nil
}

func inspectOrderedMongoBSON(value any) error {
	switch typed := value.(type) {
	case bson.D:
		for _, element := range typed {
			if element.Key == "" {
				return fmt.Errorf("MongoDB condition contains an empty key")
			}
			if element.Key == "$regex" {
				if _, ok := element.Value.(string); !ok {
					return fmt.Errorf("MongoDB regex is not a literal pattern string")
				}
			}
			if err := inspectOrderedMongoBSON(element.Value); err != nil {
				return err
			}
		}
	case bson.A:
		for _, element := range typed {
			if err := inspectOrderedMongoBSON(element); err != nil {
				return err
			}
		}
	case bson.M, map[string]any:
		return fmt.Errorf("MongoDB condition contains an unordered document")
	case []any:
		return fmt.Errorf("MongoDB condition contains a plain array")
	case bson.Regex:
		return fmt.Errorf("MongoDB condition contains an executable regex value")
	}
	return nil
}
