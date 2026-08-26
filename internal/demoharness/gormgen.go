package demoharness

import (
	"context"
	"fmt"
	"slices"

	"github.com/imbrooklyn/weave"
	"github.com/imbrooklyn/weave-adapters/gormgen"
	"github.com/imbrooklyn/weave-integration-testbed/internal/gormgenquery"
	"github.com/imbrooklyn/weave-integration-testbed/internal/scenario"
	"github.com/imbrooklyn/weave-integration-testbed/internal/sqlgorm"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
	"github.com/imbrooklyn/weave/compilertest"
)

// RunGORMGen executes canonical scenarios through a real generated DAO.
func RunGORMGen(
	ctx context.Context,
	backend testenv.Backend,
) (report scenario.Report, err error) {
	database, cleanup, err := sqlgorm.Open(ctx, backend)
	if err != nil {
		return report, err
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil && err == nil {
			err = fmt.Errorf("close %s GORM Gen database: %w", backend, cleanupErr)
		}
	}()

	profile, err := gormgenProfile(backend)
	if err != nil {
		return report, err
	}
	queries := gormgenquery.Use(database)
	records := queries.SemanticRecord
	equalityOnly, err := gormgen.NewFieldSpec[string](
		records.EqualityOnlyText,
		equalityOnlyOperators...,
	)
	if err != nil {
		return report, fmt.Errorf("create GORM Gen equality-only FieldSpec: %w", err)
	}
	compiler, err := gormgen.NewCompiler(
		profile,
		gormgen.WithFieldSpecs(equalityOnly),
	)
	if err != nil {
		return report, fmt.Errorf("create %s GORM Gen Compiler: %w", backend, err)
	}
	factory := weave.NewFactory[gormgen.Conditions, gormgen.Expression](compiler)
	harness := compilertest.Harness[gormgen.Conditions, gormgen.Expression]{
		Factory: factory,
		Fields: compilertest.Fields{
			Number:           records.NumberValue,
			Text:             records.TextValue,
			NullableNumber:   records.NullableNumber,
			NullableText:     records.NullableText,
			EqualityOnlyText: records.EqualityOnlyText,
		},
		Resolver: compiler,
		Execute: func(conditions gormgen.Conditions) ([]string, error) {
			matched, err := records.
				WithContext(ctx).
				Where(conditions...).
				Order(records.ID).
				Find()
			if err != nil {
				return nil, err
			}
			ids := make([]string, len(matched))
			for index := range matched {
				ids[index] = matched[index].ID
			}
			return ids, nil
		},
		NativeCondition: func(ids []string) gormgen.Conditions {
			return gormgen.ConditionsOf(records.ID.In(slices.Clone(ids)...))
		},
		NativeExpression: func(ids []string) gormgen.Expression {
			return records.ID.In(slices.Clone(ids)...)
		},
		DistinguishesMissing: false,
	}
	return scenario.Run(ctx, "gormgen", string(backend), harness)
}
