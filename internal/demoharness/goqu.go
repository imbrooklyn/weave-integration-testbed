package demoharness

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	sqlbuilder "github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/imbrooklyn/weave"
	weavegoqu "github.com/imbrooklyn/weave-adapters/goqu"
	"github.com/imbrooklyn/weave-integration-testbed/internal/scenario"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
	"github.com/imbrooklyn/weave/compilertest"
)

const goquTable = "semantic_records"

var postgresPlaceholder = regexp.MustCompile(`\$[0-9]+`)

// RunGoqu executes canonical scenarios as prepared database/sql queries
// against the selected real SQL backend.
func RunGoqu(
	ctx context.Context,
	backend testenv.Backend,
) (report scenario.Report, err error) {
	harness, cleanup, err := NewGoquHarness(ctx, backend)
	if err != nil {
		return report, err
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil && err == nil {
			err = fmt.Errorf("close %s goqu database: %w", backend, cleanupErr)
		}
	}()
	return scenario.Run(ctx, "goqu", string(backend), harness)
}

// NewGoquHarness opens the selected real SQL backend and returns the canonical
// compiler-test harness plus its required cleanup function.
func NewGoquHarness(
	ctx context.Context,
	backend testenv.Backend,
) (
	compilertest.Harness[weavegoqu.Expressions, weavegoqu.Expression],
	func() error,
	error,
) {
	var zero compilertest.Harness[weavegoqu.Expressions, weavegoqu.Expression]
	if ctx == nil {
		return zero, nil, fmt.Errorf("create %s goqu harness: nil context", backend)
	}
	profile, err := goquProfile(backend)
	if err != nil {
		return zero, nil, err
	}
	if _, err := goquDialect(backend); err != nil {
		return zero, nil, err
	}
	compiler, err := weavegoqu.NewCompiler(profile)
	if err != nil {
		return zero, nil, fmt.Errorf("create %s goqu Compiler: %w", backend, err)
	}
	fields, err := newGoquFields()
	if err != nil {
		return zero, nil, err
	}

	config, err := testenv.LoadSQLConfig(backend)
	if err != nil {
		return zero, nil, err
	}
	database, err := testenv.OpenSQL(config)
	if err != nil {
		return zero, nil, err
	}
	if err := testenv.WaitForSQL(ctx, backend, database, 250*time.Millisecond); err != nil {
		_ = database.Close()
		return zero, nil, err
	}

	factory := weave.NewFactory[weavegoqu.Expressions, weavegoqu.Expression](compiler)
	harness := compilertest.Harness[weavegoqu.Expressions, weavegoqu.Expression]{
		Factory:  factory,
		Fields:   fields,
		Resolver: compiler,
		Execute: func(expressions weavegoqu.Expressions) ([]string, error) {
			return executeGoqu(ctx, database, backend, expressions)
		},
		InspectCondition: func(caseName string, expressions weavegoqu.Expressions) error {
			return inspectGoquCondition(backend, caseName, expressions)
		},
		NativeCondition: func(ids []string) weavegoqu.Expressions {
			return weavegoqu.ExpressionsOf(goquIDExpression(ids))
		},
		NativeExpression: func(ids []string) weavegoqu.Expression {
			return goquIDExpression(ids)
		},
		NilLikeNativeCondition: func() weavegoqu.Expressions {
			return nil
		},
		NilLikeNativeExpression: func() weavegoqu.Expression {
			var expression *nilGoquExpression
			return expression
		},
		DistinguishesMissing: false,
	}
	return harness, database.Close, nil
}

func newGoquFields() (compilertest.Fields, error) {
	number, err := weavegoqu.NewField[int64](goquColumn("number_value"))
	if err != nil {
		return compilertest.Fields{}, fmt.Errorf("create goqu number Field: %w", err)
	}
	text, err := weavegoqu.NewField[string](goquColumn("text_value"))
	if err != nil {
		return compilertest.Fields{}, fmt.Errorf("create goqu text Field: %w", err)
	}
	nullableNumber, err := weavegoqu.NewField[int64](goquColumn("nullable_number"))
	if err != nil {
		return compilertest.Fields{}, fmt.Errorf("create goqu nullable-number Field: %w", err)
	}
	nullableText, err := weavegoqu.NewField[string](goquColumn("nullable_text"))
	if err != nil {
		return compilertest.Fields{}, fmt.Errorf("create goqu nullable-text Field: %w", err)
	}
	equalityOnlyText, err := weavegoqu.NewFieldWithOperators[string](
		goquColumn("equality_only_text"),
		equalityOnlyOperators...,
	)
	if err != nil {
		return compilertest.Fields{}, fmt.Errorf("create goqu equality-only Field: %w", err)
	}
	return compilertest.Fields{
		Number:           number,
		Text:             text,
		NullableNumber:   nullableNumber,
		NullableText:     nullableText,
		EqualityOnlyText: equalityOnlyText,
	}, nil
}

func goquColumn(name string) exp.IdentifierExpression {
	return sqlbuilder.T(goquTable).Col(name)
}

func goquIDExpression(ids []string) exp.Expression {
	values := make(exp.Vals, len(ids))
	for index := range ids {
		values[index] = ids[index]
	}
	return goquColumn("id").In(values)
}

// RenderGoquPrepared renders the exact prepared SELECT used by the goqu
// harness. The returned argument slice remains separate from the SQL template.
func RenderGoquPrepared(
	backend testenv.Backend,
	expressions weavegoqu.Expressions,
) (string, []any, error) {
	if expressions == nil {
		return "", nil, fmt.Errorf("render %s goqu query: nil Expressions", backend)
	}
	dialect, err := goquDialect(backend)
	if err != nil {
		return "", nil, err
	}
	query, arguments, err := sqlbuilder.
		Dialect(dialect).
		From(sqlbuilder.T(goquTable)).
		Select(goquColumn("id")).
		Where(expressions...).
		Order(goquColumn("id").Asc()).
		Prepared(true).
		ToSQL()
	if err != nil {
		return "", nil, fmt.Errorf("render %s prepared goqu query: %w", backend, err)
	}
	return query, arguments, nil
}

func executeGoqu(
	ctx context.Context,
	database *sql.DB,
	backend testenv.Backend,
	expressions weavegoqu.Expressions,
) ([]string, error) {
	query, arguments, err := RenderGoquPrepared(backend, expressions)
	if err != nil {
		return nil, err
	}
	rows, err := database.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("execute %s prepared goqu query: %w", backend, err)
	}
	defer rows.Close()

	ids := make([]string, 0, 6)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan %s goqu result ID: %w", backend, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s goqu result IDs: %w", backend, err)
	}
	return ids, nil
}

func inspectGoquCondition(
	backend testenv.Backend,
	caseName string,
	expressions weavegoqu.Expressions,
) error {
	query, arguments, err := RenderGoquPrepared(backend, expressions)
	if err != nil {
		return err
	}
	placeholderCount := strings.Count(query, "?")
	quotedTable := "`" + goquTable + "`"
	if backend == testenv.PostgreSQL {
		placeholderCount = len(postgresPlaceholder.FindAllString(query, -1))
		quotedTable = `"` + goquTable + `"`
		if strings.Contains(query, "?") {
			return fmt.Errorf("%s prepared template contains an unnumbered placeholder", backend)
		}
	}
	if placeholderCount != len(arguments) {
		return fmt.Errorf(
			"%s prepared template has %d placeholders for %d arguments",
			backend,
			placeholderCount,
			len(arguments),
		)
	}
	if !strings.Contains(query, quotedTable) {
		return fmt.Errorf("%s prepared template lacks the quoted fixture table", backend)
	}
	for index, argument := range arguments {
		if _, ok := argument.(exp.IdentifierExpression); ok {
			return fmt.Errorf("%s prepared arguments contain an identifier", backend)
		}
		if _, ok := argument.(exp.Expression); ok {
			return fmt.Errorf("%s prepared arguments contain an expression", backend)
		}
		var boundText string
		switch typed := argument.(type) {
		case string:
			boundText = typed
		case []byte:
			boundText = string(typed)
		}
		if boundText != "" && strings.Contains(query, boundText) {
			return fmt.Errorf(
				"%s prepared template discloses argument %d in case %q",
				backend,
				index,
				caseName,
			)
		}
	}
	return nil
}

type nilGoquExpression struct{}

func (*nilGoquExpression) Clone() exp.Expression {
	return (*nilGoquExpression)(nil)
}

func (expression *nilGoquExpression) Expression() exp.Expression {
	return expression
}
