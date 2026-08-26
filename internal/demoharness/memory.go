// Package demoharness binds public Adapter APIs to the canonical scenarios
// shared by runnable demos and integration tests.
package demoharness

import (
	"context"
	"fmt"

	"github.com/imbrooklyn/weave"
	"github.com/imbrooklyn/weave-adapters/memory"
	"github.com/imbrooklyn/weave-integration-testbed/internal/scenario"
	"github.com/imbrooklyn/weave/compilertest"
)

// RunMemory executes the in-process reference match-set.
func RunMemory(ctx context.Context) (scenario.Report, error) {
	harness, err := memoryHarness(ctx)
	if err != nil {
		return scenario.Report{}, err
	}
	return scenario.Run(ctx, "memory", "reference", harness)
}

func memoryHarness(ctx context.Context) (
	compilertest.Harness[
		memory.Condition[compilertest.Record],
		memory.Expression[compilertest.Record],
	],
	error,
) {
	var zero compilertest.Harness[
		memory.Condition[compilertest.Record],
		memory.Expression[compilertest.Record],
	]
	number, err := memory.NewField(
		"number",
		func(record compilertest.Record) (int64, memory.State) {
			return record.Number, memory.StateValue
		},
		memory.OrderedSemantics[int64](),
	)
	if err != nil {
		return zero, fmt.Errorf("create memory number field: %w", err)
	}
	text, err := memory.NewField(
		"text",
		func(record compilertest.Record) (string, memory.State) {
			return record.Text, memory.StateValue
		},
		memory.StringSemantics(),
	)
	if err != nil {
		return zero, fmt.Errorf("create memory text field: %w", err)
	}
	nullableNumber, err := memory.NewField(
		"nullable_number",
		func(record compilertest.Record) (int64, memory.State) {
			if !record.NullableNumberPresent {
				return 0, memory.StateMissing
			}
			if record.NullableNumber == nil {
				return 0, memory.StateNull
			}
			return *record.NullableNumber, memory.StateValue
		},
		memory.OrderedSemantics[int64](),
	)
	if err != nil {
		return zero, fmt.Errorf("create memory nullable-number field: %w", err)
	}
	nullableText, err := memory.NewField(
		"nullable_text",
		func(record compilertest.Record) (string, memory.State) {
			if !record.NullableTextPresent {
				return "", memory.StateMissing
			}
			if record.NullableText == nil {
				return "", memory.StateNull
			}
			return *record.NullableText, memory.StateValue
		},
		memory.StringSemantics(),
	)
	if err != nil {
		return zero, fmt.Errorf("create memory nullable-text field: %w", err)
	}
	equalityOnlyText, err := memory.NewField(
		"equality_only_text",
		func(record compilertest.Record) (string, memory.State) {
			return record.Text, memory.StateValue
		},
		memory.ComparableSemantics[string](),
	)
	if err != nil {
		return zero, fmt.Errorf("create memory equality-only field: %w", err)
	}

	records := compilertest.Records()
	compiler := memory.NewCompiler[compilertest.Record]()
	factory := weave.NewFactory[
		memory.Condition[compilertest.Record],
		memory.Expression[compilertest.Record],
	](compiler)
	return compilertest.Harness[
		memory.Condition[compilertest.Record],
		memory.Expression[compilertest.Record],
	]{
		Factory: factory,
		Fields: compilertest.Fields{
			Number:           number,
			Text:             text,
			NullableNumber:   nullableNumber,
			NullableText:     nullableText,
			EqualityOnlyText: equalityOnlyText,
		},
		Resolver: compiler,
		Execute: func(condition memory.Condition[compilertest.Record]) ([]string, error) {
			ids := make([]string, 0, len(records))
			for _, record := range records {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				matched, err := condition.Match(record)
				if err != nil {
					return nil, err
				}
				if matched {
					ids = append(ids, record.ID)
				}
			}
			return ids, nil
		},
		NativeCondition: func(ids []string) memory.Condition[compilertest.Record] {
			matches := idSet(ids)
			return func(record compilertest.Record) (bool, error) {
				_, matched := matches[record.ID]
				return matched, nil
			}
		},
		NativeExpression: func(ids []string) memory.Expression[compilertest.Record] {
			matches := idSet(ids)
			return func(record compilertest.Record) (bool, error) {
				_, matched := matches[record.ID]
				return matched, nil
			}
		},
		DistinguishesMissing: true,
	}, nil
}

func idSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}
