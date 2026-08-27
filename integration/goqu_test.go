//go:build integration

package integration

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	sqlbuilder "github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	weavegoqu "github.com/imbrooklyn/weave-adapters/goqu"
	"github.com/imbrooklyn/weave-integration-testbed/internal/demoharness"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
	"github.com/imbrooklyn/weave/compilertest"
)

func TestGoquCompilerContractAgainstRealSQL(t *testing.T) {
	for _, backend := range testenv.SQLBackends() {
		t.Run(string(backend), func(t *testing.T) {
			resetFixture(t, backend)
			ctx, cancel := context.WithTimeout(context.Background(), backendTimeout)
			defer cancel()
			harness, cleanup, err := demoharness.NewGoquHarness(ctx, backend)
			if err != nil {
				t.Fatalf("create %s goqu harness: %v", backend, err)
			}
			defer func() {
				if err := cleanup(); err != nil {
					t.Errorf("close %s goqu harness: %v", backend, err)
				}
			}()
			compilertest.Run(t, harness)
		})
	}
}

func TestGoquPreparedDeterminismConcurrencyAndTopLevelShallowClone(t *testing.T) {
	for _, backend := range testenv.SQLBackends() {
		t.Run(string(backend), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), backendTimeout)
			defer cancel()
			harness, cleanup, err := demoharness.NewGoquHarness(ctx, backend)
			if err != nil {
				t.Fatalf("create %s goqu harness: %v", backend, err)
			}
			defer func() {
				if err := cleanup(); err != nil {
					t.Errorf("close %s goqu harness: %v", backend, err)
				}
			}()

			predicate, err := harness.Factory.New().
				GTE(harness.Fields.Number, int64(2)).
				Contains(harness.Fields.Text, "prefix %_!").
				Predicate()
			if err != nil {
				t.Fatal(err)
			}
			wantExpressions, err := harness.Factory.Compile(predicate)
			if err != nil {
				t.Fatal(err)
			}
			wantQuery, wantArguments, err := demoharness.RenderGoquPrepared(
				backend,
				wantExpressions,
			)
			if err != nil {
				t.Fatal(err)
			}

			mutated, err := harness.Factory.Compile(predicate)
			if err != nil {
				t.Fatal(err)
			}
			mutated[0] = sqlbuilder.L("caller_mutation = 1")
			again, err := harness.Factory.Compile(predicate)
			if err != nil {
				t.Fatal(err)
			}
			againQuery, againArguments, err := demoharness.RenderGoquPrepared(backend, again)
			if err != nil {
				t.Fatal(err)
			}
			if againQuery != wantQuery || !reflect.DeepEqual(againArguments, wantArguments) {
				t.Fatal("Compile reused caller-visible top-level expression storage")
			}

			const workers = 32
			errorsFound := make(chan error, workers)
			var wait sync.WaitGroup
			for range workers {
				wait.Add(1)
				go func() {
					defer wait.Done()
					expressions, err := harness.Factory.Compile(predicate)
					if err != nil {
						errorsFound <- err
						return
					}
					query, arguments, err := demoharness.RenderGoquPrepared(backend, expressions)
					if err != nil {
						errorsFound <- err
						return
					}
					if query != wantQuery || !reflect.DeepEqual(arguments, wantArguments) {
						errorsFound <- errors.New("concurrent prepared output changed")
					}
				}()
			}
			wait.Wait()
			close(errorsFound)
			for err := range errorsFound {
				t.Fatalf("concurrent Compile/Render error = %v", err)
			}

			borrowed := &borrowedGoquExpression{label: "borrowed"}
			native := weavegoqu.ExpressionsOf(borrowed)
			nativePredicate, err := harness.Factory.New().Native(native).Predicate()
			if err != nil {
				t.Fatal(err)
			}
			native[0] = sqlbuilder.L("caller_input_mutation = 1")
			first, err := harness.Factory.Compile(nativePredicate)
			if err != nil {
				t.Fatal(err)
			}
			if len(first) != 1 || first[0] != borrowed {
				t.Fatal("Native did not preserve borrowed nested expression identity")
			}
			first[0] = sqlbuilder.L("caller_output_mutation = 1")
			second, err := harness.Factory.Compile(nativePredicate)
			if err != nil {
				t.Fatal(err)
			}
			if len(second) != 1 || second[0] != borrowed {
				t.Fatal("Compile did not return an independent shallow top-level slice")
			}
		})
	}
}

type borrowedGoquExpression struct {
	label string
}

func (expression *borrowedGoquExpression) Clone() exp.Expression {
	if expression == nil {
		return (*borrowedGoquExpression)(nil)
	}
	clone := *expression
	return &clone
}

func (expression *borrowedGoquExpression) Expression() exp.Expression {
	return expression
}
