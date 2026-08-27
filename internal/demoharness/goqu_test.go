package demoharness

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	sqlbuilder "github.com/doug-martin/goqu/v9"
	"github.com/imbrooklyn/weave"
	weavegoqu "github.com/imbrooklyn/weave-adapters/goqu"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
	"github.com/imbrooklyn/weave/compilertest"
)

func TestGoquSQLProfileWiring(t *testing.T) {
	tests := []struct {
		backend     testenv.Backend
		wantProfile weavegoqu.Profile
		wantDialect string
	}{
		{backend: testenv.MySQL, wantProfile: weavegoqu.MySQL, wantDialect: "mysql"},
		{backend: testenv.PostgreSQL, wantProfile: weavegoqu.PostgreSQL, wantDialect: "postgres"},
	}
	for _, test := range tests {
		t.Run(string(test.backend), func(t *testing.T) {
			profile, err := goquProfile(test.backend)
			if err != nil || profile != test.wantProfile {
				t.Fatalf("goquProfile() = (%v, %v), want (%v, nil)", profile, err, test.wantProfile)
			}
			dialect, err := goquDialect(test.backend)
			if err != nil || dialect != test.wantDialect {
				t.Fatalf("goquDialect() = (%q, %v), want (%q, nil)", dialect, err, test.wantDialect)
			}
		})
	}

	if _, err := goquProfile(testenv.Backend("sqlite")); err == nil {
		t.Fatal("goquProfile(sqlite) error = nil")
	}
	if _, err := goquDialect(testenv.Backend("sqlite")); err == nil {
		t.Fatal("goquDialect(sqlite) error = nil")
	}
}

func TestGoquPreparedBoundaryKeepsIdentifiersSafeAndValuesBound(t *testing.T) {
	const payload = "private' OR 1=1 --"
	fields, err := newGoquFields()
	if err != nil {
		t.Fatal(err)
	}
	for _, backend := range testenv.SQLBackends() {
		t.Run(string(backend), func(t *testing.T) {
			profile, err := goquProfile(backend)
			if err != nil {
				t.Fatal(err)
			}
			factory, err := weavegoqu.NewFactory(profile)
			if err != nil {
				t.Fatal(err)
			}
			expressions, err := factory.New().
				EQ(fields.Text, payload).
				Contains(fields.Text, compilertest.LiteralSpecialText).
				Build()
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			query, arguments, err := RenderGoquPrepared(backend, expressions)
			if err != nil {
				t.Fatalf("RenderGoquPrepared() error = %v", err)
			}
			for _, secret := range []string{payload, compilertest.LiteralSpecialText} {
				if strings.Contains(query, secret) {
					t.Fatalf("prepared SQL contains query value %q", secret)
				}
			}
			if len(arguments) != 2 || arguments[0] != payload {
				t.Fatalf("prepared arguments = %#v, want payload plus LIKE pattern", arguments)
			}
			pattern, ok := arguments[1].(string)
			if !ok || !strings.Contains(pattern, "!%") ||
				!strings.Contains(pattern, "!_") || !strings.Contains(pattern, "!!") {
				t.Fatalf("literal LIKE argument = %#v, want escaped %%, _, and !", arguments[1])
			}
			if err := inspectGoquCondition(backend, "prepared boundary", expressions); err != nil {
				t.Fatalf("inspectGoquCondition() error = %v", err)
			}
		})
	}

	if _, err := weavegoqu.NewField[string](sqlbuilder.C("id; DROP TABLE semantic_records")); !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("unsafe NewField() error = %v, want ErrInvalidField", err)
	}
	factory, err := weavegoqu.NewFactory(weavegoqu.MySQL)
	if err != nil {
		t.Fatal(err)
	}
	expressions, err := factory.New().
		EQ(sqlbuilder.L("private_fragment"), payload).
		Build()
	if expressions != nil || !errors.Is(err, weave.ErrInvalidField) {
		t.Fatalf("raw field Build() = (%#v, %v), want nil ErrInvalidField", expressions, err)
	}
	for _, secret := range []string{"private_fragment", payload} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("compile error discloses %q: %v", secret, err)
		}
	}
}

func TestRenderGoquPreparedRejectsInvalidInputs(t *testing.T) {
	if query, arguments, err := RenderGoquPrepared(testenv.MySQL, nil); query != "" || arguments != nil || err == nil {
		t.Fatalf("RenderGoquPrepared(nil) = (%q, %#v, %v)", query, arguments, err)
	}
	if query, arguments, err := RenderGoquPrepared(
		testenv.Backend("sqlite"),
		weavegoqu.Expressions{},
	); query != "" || arguments != nil || err == nil {
		t.Fatalf("RenderGoquPrepared(sqlite) = (%q, %#v, %v)", query, arguments, err)
	}
}

func TestGoquPreparedOutputIsStableWithoutDatabase(t *testing.T) {
	fields, err := newGoquFields()
	if err != nil {
		t.Fatal(err)
	}
	for _, backend := range testenv.SQLBackends() {
		profile, err := goquProfile(backend)
		if err != nil {
			t.Fatal(err)
		}
		factory, err := weavegoqu.NewFactory(profile)
		if err != nil {
			t.Fatal(err)
		}
		predicate, err := factory.New().
			GTE(fields.Number, int64(2)).
			Contains(fields.Text, "prefix %_!").
			Predicate()
		if err != nil {
			t.Fatal(err)
		}
		wantExpressions, err := factory.Compile(predicate)
		if err != nil {
			t.Fatal(err)
		}
		wantQuery, wantArguments, err := RenderGoquPrepared(backend, wantExpressions)
		if err != nil {
			t.Fatal(err)
		}
		for iteration := range 32 {
			expressions, err := factory.Compile(predicate)
			if err != nil {
				t.Fatalf("Compile(%d) error = %v", iteration, err)
			}
			query, arguments, err := RenderGoquPrepared(backend, expressions)
			if err != nil {
				t.Fatalf("Render(%d) error = %v", iteration, err)
			}
			if query != wantQuery || !reflect.DeepEqual(arguments, wantArguments) {
				t.Fatalf("prepared output changed at iteration %d", iteration)
			}
		}
	}
}
