package demoharness

import (
	"context"
	"fmt"

	"github.com/imbrooklyn/weave"
	weavegorm "github.com/imbrooklyn/weave-adapters/gorm"
	"github.com/imbrooklyn/weave-integration-testbed/internal/gormgenmodel"
	"github.com/imbrooklyn/weave-integration-testbed/internal/scenario"
	"github.com/imbrooklyn/weave-integration-testbed/internal/sqlgorm"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
	"github.com/imbrooklyn/weave/compilertest"
	"gorm.io/gorm/clause"
)

// RunGORM executes canonical scenarios through public typed Fields and
// traditional GORM DB.Where.
func RunGORM(
	ctx context.Context,
	backend testenv.Backend,
) (report scenario.Report, err error) {
	database, cleanup, err := sqlgorm.Open(ctx, backend)
	if err != nil {
		return report, err
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil && err == nil {
			err = fmt.Errorf("close %s GORM database: %w", backend, cleanupErr)
		}
	}()

	profile, err := gormProfile(backend)
	if err != nil {
		return report, err
	}
	const table = gormgenmodel.TableNameSemanticRecord
	number := weavegorm.MustQualifiedField[int64](table, "number_value")
	text := weavegorm.MustQualifiedField[string](table, "text_value")
	nullableNumber := weavegorm.MustQualifiedField[int64](table, "nullable_number")
	nullableText := weavegorm.MustQualifiedField[string](table, "nullable_text")
	equalityOnlyText := weavegorm.MustQualifiedField[string](
		table,
		"equality_only_text",
		weavegorm.WithOperators(equalityOnlyOperators...),
	)
	compiler, err := weavegorm.NewCompiler(profile)
	if err != nil {
		return report, fmt.Errorf("create %s GORM Compiler: %w", backend, err)
	}
	factory := weave.NewFactory[weavegorm.Condition, weavegorm.Expression](compiler)
	harness := compilertest.Harness[weavegorm.Condition, weavegorm.Expression]{
		Factory: factory,
		Fields: compilertest.Fields{
			Number:           number,
			Text:             text,
			NullableNumber:   nullableNumber,
			NullableText:     nullableText,
			EqualityOnlyText: equalityOnlyText,
		},
		Resolver: compiler,
		Execute: func(condition weavegorm.Condition) ([]string, error) {
			var matched []gormgenmodel.SemanticRecord
			result := database.WithContext(ctx).Where(condition).Find(&matched)
			if result.Error != nil {
				return nil, result.Error
			}
			ids := make([]string, len(matched))
			for index := range matched {
				ids[index] = matched[index].ID
			}
			return ids, nil
		},
		NativeCondition:      idCondition,
		NativeExpression:     idCondition,
		DistinguishesMissing: false,
	}
	return scenario.Run(ctx, "gorm", string(backend), harness)
}

func idCondition(ids []string) clause.Expression {
	values := make([]any, len(ids))
	for index := range ids {
		values[index] = ids[index]
	}
	return clause.IN{
		Column: clause.Column{
			Table: gormgenmodel.TableNameSemanticRecord,
			Name:  "id",
		},
		Values: values,
	}
}
