//go:build integration

package integration

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/imbrooklyn/weave"
	weaveldap "github.com/imbrooklyn/weave-adapters/ldap"
	"github.com/imbrooklyn/weave-integration-testbed/internal/demoharness"
	"github.com/imbrooklyn/weave-integration-testbed/internal/fixture"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
	"github.com/imbrooklyn/weave/compilertest"
)

func TestLDAPCompilerContractAgainstRealServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), backendTimeout)
	defer cancel()
	harness, server, err := demoharness.NewLDAPHarness(ctx)
	if err != nil {
		t.Fatalf("create LDAP harness: %v", err)
	}
	if server.Vendor != "OpenLDAP" || server.Version != testenv.OpenLDAPVersion {
		t.Fatalf("LDAP server identity = %#v", server)
	}
	t.Logf("real LDAP compiler contract server: %s %s", server.Vendor, server.Version)
	compilertest.Run(t, harness)
}

func TestLDAPCardinalityAbsenceEmptyAndMatchingRules(t *testing.T) {
	runtime := resetLDAPFixture(t)

	equality, err := runtime.adapter.Factory.New().
		EQ(runtime.adapter.Fields.Number, int64(2)).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	assertLDAPIDs(t, runtime, fixture.LDAPRecordsDN, equality, []string{"r02"})

	multiExpression, err := runtime.adapter.Factory.New().
		Expr("(" + fixture.LDAPTags + "=shared)").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	assertLDAPIDs(
		t,
		runtime,
		fixture.LDAPRecordsDN,
		multiExpression,
		[]string{"r01", "r02"},
	)

	empty, err := runtime.adapter.Factory.New().
		EQ(runtime.adapter.Fields.EmptyIA5, "").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(empty.String(), "=)") {
		t.Fatal("empty IA5 value was not emitted as an empty equality assertion")
	}
	assertLDAPIDs(t, runtime, fixture.LDAPProbesDN, empty, []string{"p04"})
	present, err := runtime.adapter.Factory.New().
		NotNull(runtime.adapter.Fields.EmptyIA5).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	assertLDAPIDs(t, runtime, fixture.LDAPProbesDN, present, []string{"p04"})

	ordered, err := runtime.adapter.Factory.New().
		GTE(runtime.adapter.Fields.Number, int64(104)).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	assertLDAPIDs(t, runtime, fixture.LDAPProbesDN, ordered, []string{"p04", "p05"})

	substring, err := runtime.adapter.Factory.New().
		Contains(runtime.adapter.Fields.Text, "*()\\").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	for _, escaped := range []string{"\\2a", "\\28", "\\29", "\\5c"} {
		if !strings.Contains(substring.String(), escaped) {
			t.Fatalf("substring filter omits escape %q", escaped)
		}
	}
	assertLDAPIDs(t, runtime, fixture.LDAPProbesDN, substring, []string{"p01"})
}

func TestLDAPOrdinaryNegationUsesBinaryMatchSets(t *testing.T) {
	runtime := resetLDAPFixture(t)

	neq, err := runtime.adapter.Factory.New().
		NEQ(runtime.adapter.Fields.NullableNumber, int64(2)).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	assertLDAPIDs(t, runtime, fixture.LDAPRecordsDN, neq, []string{"r01", "r05"})

	notIn, err := runtime.adapter.Factory.New().
		NotIn(runtime.adapter.Fields.NullableNumber, []int64{2, 5}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	assertLDAPIDs(t, runtime, fixture.LDAPRecordsDN, notIn, []string{"r01"})

	notOrdinary, err := runtime.adapter.Factory.New().
		NoneOf(func(group *weaveldap.Group) {
			group.EQ(runtime.adapter.Fields.NullableNumber, int64(2))
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	assertLDAPIDs(
		t,
		runtime,
		fixture.LDAPRecordsDN,
		notOrdinary,
		[]string{"r01", "r03", "r04", "r05"},
	)
}

func TestLDAPEscapingUnicodeNULAndInjectionLikeValues(t *testing.T) {
	runtime := resetLDAPFixture(t)

	octets, err := runtime.adapter.Factory.New().
		EQ(runtime.adapter.Fields.Octets, []byte(fixture.LDAPNULOctets)).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(octets.String(), '\x00') ||
		!strings.Contains(octets.String(), "\\00") {
		t.Fatal("NUL assertion value was not represented by an RFC 4515 escape")
	}
	assertLDAPIDs(t, runtime, fixture.LDAPProbesDN, octets, []string{"p03"})

	unicodeFilter, err := runtime.adapter.Factory.New().
		Contains(runtime.adapter.Fields.Text, "世界").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unicodeFilter.String(), "\\e4\\b8\\96\\e7\\95\\8c") {
		t.Fatal("Unicode assertion was not canonicalized as escaped UTF-8 bytes")
	}
	assertLDAPIDs(t, runtime, fixture.LDAPProbesDN, unicodeFilter, []string{"p01"})

	injection, err := runtime.adapter.Factory.New().
		EQ(runtime.adapter.Fields.Text, fixture.LDAPInjectionText).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(injection.String(), fixture.LDAPInjectionText) ||
		strings.Contains(injection.String(), "(|("+fixture.LDAPRecordID) {
		t.Fatal("injection-like assertion changed the filter grammar")
	}
	for _, escaped := range []string{"\\2a", "\\29", "\\28"} {
		if !strings.Contains(injection.String(), escaped) {
			t.Fatalf("injection-like filter omits escape %q", escaped)
		}
	}
	assertLDAPIDs(t, runtime, fixture.LDAPProbesDN, injection, []string{"p02"})
}

func TestLDAPFourLogicDeepNativeExprAndStableDNs(t *testing.T) {
	runtime := resetLDAPFixture(t)

	tests := []struct {
		name  string
		build func() (weaveldap.Filter, error)
		want  []string
	}{
		{
			name: "all of",
			build: func() (weaveldap.Filter, error) {
				return runtime.adapter.Factory.New().AllOf(func(group *weaveldap.Group) {
					group.GTE(runtime.adapter.Fields.Number, int64(2)).
						LTE(runtime.adapter.Fields.Number, int64(4))
				}).Build()
			},
			want: []string{"r02", "r03", "r04"},
		},
		{
			name: "any of",
			build: func() (weaveldap.Filter, error) {
				return runtime.adapter.Factory.New().AnyOf(func(group *weaveldap.Group) {
					group.EQ(runtime.adapter.Fields.Number, int64(1)).
						EQ(runtime.adapter.Fields.Number, int64(6))
				}).Build()
			},
			want: []string{"r01", "r06"},
		},
		{
			name: "none of",
			build: func() (weaveldap.Filter, error) {
				return runtime.adapter.Factory.New().NoneOf(func(group *weaveldap.Group) {
					group.EQ(runtime.adapter.Fields.NullableNumber, int64(2))
				}).Build()
			},
			want: []string{"r01", "r03", "r04", "r05"},
		},
		{
			name: "not all of",
			build: func() (weaveldap.Filter, error) {
				return runtime.adapter.Factory.New().NotAllOf(func(group *weaveldap.Group) {
					group.GTE(runtime.adapter.Fields.Number, int64(2)).
						LTE(runtime.adapter.Fields.Number, int64(5))
				}).Build()
			},
			want: []string{"r01", "r06"},
		},
		{
			name: "three-level logic",
			build: func() (weaveldap.Filter, error) {
				return runtime.adapter.Factory.New().AllOf(func(levelOne *weaveldap.Group) {
					levelOne.AnyOf(func(levelTwo *weaveldap.Group) {
						levelTwo.NoneOf(func(levelThree *weaveldap.Group) {
							levelThree.EQ(runtime.adapter.Fields.Number, int64(1)).
								EQ(runtime.adapter.Fields.Number, int64(2))
						}).EQ(runtime.adapter.Fields.Number, int64(6))
					}).GTE(runtime.adapter.Fields.Number, int64(3))
				}).Build()
			},
			want: []string{"r03", "r04", "r05", "r06"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter, err := test.build()
			if err != nil {
				t.Fatal(err)
			}
			assertLDAPIDs(t, runtime, fixture.LDAPRecordsDN, filter, test.want)
		})
	}

	nativeValue, err := weaveldap.ParseFilter(
		runtime.adapter.Schema,
		"(|("+fixture.LDAPRecordID+"=r02)("+fixture.LDAPRecordID+"=r04))",
	)
	if err != nil {
		t.Fatal(err)
	}
	native, err := runtime.adapter.Factory.New().
		Native(nativeValue).
		GTE(runtime.adapter.Fields.Number, int64(3)).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	assertLDAPIDs(t, runtime, fixture.LDAPRecordsDN, native, []string{"r04"})

	expression, err := runtime.adapter.Factory.New().AnyOf(func(group *weaveldap.Group) {
		group.Expr("(|("+fixture.LDAPRecordID+"=r01)("+fixture.LDAPRecordID+"=r03))").
			EQ(runtime.adapter.Fields.Number, int64(6))
	}).Build()
	if err != nil {
		t.Fatal(err)
	}
	assertLDAPIDs(t, runtime, fixture.LDAPRecordsDN, expression, []string{"r01", "r03", "r06"})

	root, err := runtime.adapter.Factory.New().Build()
	if err != nil {
		t.Fatal(err)
	}
	dns, err := testenv.QueryLDAPDNs(
		runtime.ctx,
		runtime.config,
		fixture.LDAPRecordsDN,
		root.String(),
	)
	if err != nil {
		t.Fatal(err)
	}
	wantDNs := fixture.LDAPRecordDNs()
	slices.Sort(wantDNs)
	if !slices.Equal(dns, wantDNs) {
		t.Fatalf("LDAP DNs = %v, want %v", dns, wantDNs)
	}
}

func TestLDAPUnsupportedOperationsAreStructuredAndNeverApproximated(t *testing.T) {
	runtime := resetLDAPFixture(t)
	tests := []struct {
		name  string
		build func() (weaveldap.Filter, error)
		code  weave.ErrorCode
		phase weave.ErrorPhase
	}{
		{
			name: "is null",
			build: func() (weaveldap.Filter, error) {
				return runtime.adapter.Factory.New().IsNull(runtime.adapter.Fields.NullableNumber).Build()
			},
			code: weave.CodeUnsupportedOperator, phase: weave.PhasePreflight,
		},
		{
			name: "strict less than",
			build: func() (weaveldap.Filter, error) {
				return runtime.adapter.Factory.New().LT(runtime.adapter.Fields.Number, int64(2)).Build()
			},
			code: weave.CodeUnsupportedOperator, phase: weave.PhasePreflight,
		},
		{
			name: "strict greater than",
			build: func() (weaveldap.Filter, error) {
				return runtime.adapter.Factory.New().GT(runtime.adapter.Fields.Number, int64(2)).Build()
			},
			code: weave.CodeUnsupportedOperator, phase: weave.PhasePreflight,
		},
		{
			name: "multi-valued standard field",
			build: func() (weaveldap.Filter, error) {
				return runtime.adapter.Factory.New().EQ(runtime.adapter.Fields.Tags, "one").Build()
			},
			code: weave.CodeOperatorNotApplicable, phase: weave.PhaseValidate,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter, err := test.build()
			if filter.Valid() || !errors.Is(err, weave.ErrCompile) {
				t.Fatalf("Build() = (%q, %v)", filter.String(), err)
			}
			var detail *weave.Error
			if !errors.As(err, &detail) || detail.Code != test.code || detail.Phase != test.phase {
				t.Fatalf("structured error = %#v, want (%s, %s)", detail, test.code, test.phase)
			}
			for _, secret := range []string{
				fixture.LDAPInjectionText,
				runtime.config.AdminPassword,
				runtime.config.BindDN(),
			} {
				if strings.Contains(err.Error(), secret) {
					t.Fatal("compile error exposed filter, value, or credential data")
				}
			}
		})
	}
}

func TestLDAPCompileAndRealSearchAreDeterministicAndConcurrent(t *testing.T) {
	runtime := resetLDAPFixture(t)
	predicate, err := runtime.adapter.Factory.New().
		GTE(runtime.adapter.Fields.Number, int64(2)).
		AnyOf(func(group *weaveldap.Group) {
			group.Contains(runtime.adapter.Fields.Text, "prefix").
				In(runtime.adapter.Fields.Number, []int64{2, 6})
		}).
		Predicate()
	if err != nil {
		t.Fatal(err)
	}
	want, err := runtime.adapter.Factory.Compile(predicate)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{"r02", "r03", "r04", "r06"}
	assertLDAPIDs(t, runtime, fixture.LDAPRecordsDN, want, wantIDs)

	const workers = 16
	errorsFound := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			filter, err := runtime.adapter.Factory.Compile(predicate)
			if err != nil {
				errorsFound <- err
				return
			}
			if filter.String() != want.String() {
				errorsFound <- errors.New("concurrent Compile changed the canonical filter")
				return
			}
			ids, err := testenv.QueryLDAPIDs(
				runtime.ctx,
				runtime.config,
				fixture.LDAPRecordsDN,
				filter.String(),
			)
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
		t.Fatal(err)
	}
	t.Logf("OpenLDAP %s returned stable IDs in %d concurrent compile/search workers", runtime.server.Version, workers)
}

func TestLDAPDriverFailuresAreRedacted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), backendTimeout)
	defer cancel()
	config, err := testenv.LoadLDAPConfig()
	if err != nil {
		t.Fatal(err)
	}

	wrong := config
	wrong.AdminUser = "driver-private-user"
	wrong.AdminPassword = "driver-private-password"
	connection, bindErr := testenv.OpenLDAP(wrong)
	testenv.CloseLDAP(connection)
	if bindErr == nil {
		t.Fatal("LDAP bind with private test credentials unexpectedly succeeded")
	}
	assertLDAPErrorRedacted(t, bindErr, wrong.AdminUser, wrong.AdminPassword, wrong.BindDN())

	malformed := "(" + fixture.LDAPText + "=driver-private-filter"
	_, searchErr := testenv.QueryLDAPIDs(
		ctx,
		config,
		fixture.LDAPRecordsDN,
		malformed,
	)
	if searchErr == nil {
		t.Fatal("malformed LDAP filter unexpectedly executed")
	}
	assertLDAPErrorRedacted(
		t,
		searchErr,
		malformed,
		"driver-private-filter",
		config.AdminPassword,
		config.BindDN(),
	)
}

type ldapRuntime struct {
	ctx     context.Context
	config  testenv.LDAPConfig
	server  testenv.LDAPServerInfo
	adapter demoharness.LDAPAdapter
}

func resetLDAPFixture(t *testing.T) ldapRuntime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backendTimeout)
	t.Cleanup(cancel)
	config, err := testenv.LoadLDAPConfig()
	if err != nil {
		t.Fatal(err)
	}
	connection, err := testenv.WaitForLDAP(ctx, config, 250*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	server, err := testenv.ReadLDAPServerInfo(connection)
	if err == nil {
		err = testenv.ResetLDAP(connection)
	}
	testenv.CloseLDAP(connection)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := demoharness.NewLDAPAdapter()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("real LDAP fixture reset on %s %s", server.Vendor, server.Version)
	return ldapRuntime{ctx: ctx, config: config, server: server, adapter: adapter}
}

func assertLDAPIDs(
	t *testing.T,
	runtime ldapRuntime,
	baseDN string,
	filter weaveldap.Filter,
	want []string,
) {
	t.Helper()
	if !filter.Valid() {
		t.Fatal("query received a zero LDAP Filter")
	}
	ids, err := testenv.QueryLDAPIDs(
		runtime.ctx,
		runtime.config,
		baseDN,
		filter.String(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.CompareIDs(ids, want); err != nil {
		t.Fatal(err)
	}
}

func assertLDAPErrorRedacted(t *testing.T, err error, secrets ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a redacted LDAP error")
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(err.Error(), secret) {
			t.Fatal("LDAP error exposed filter or credential data")
		}
	}
}
