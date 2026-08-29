package demoharness

import (
	"context"
	"fmt"
	"strings"
	"time"

	ldapv3 "github.com/go-ldap/ldap/v3"
	"github.com/imbrooklyn/weave"
	weaveldap "github.com/imbrooklyn/weave-adapters/ldap"
	"github.com/imbrooklyn/weave-integration-testbed/internal/fixture"
	"github.com/imbrooklyn/weave-integration-testbed/internal/scenario"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
	"github.com/imbrooklyn/weave/compilertest"
)

const (
	caseExactMatchOID          = "2.5.13.5"
	caseExactOrderingMatchOID  = "2.5.13.6"
	caseExactSubstringMatchOID = "2.5.13.7"
	integerMatchOID            = "2.5.13.14"
	integerOrderingMatchOID    = "2.5.13.15"
	caseExactIA5MatchOID       = "1.3.6.1.4.1.1466.109.114.1"
	octetStringMatchOID        = "2.5.13.17"
)

// LDAPFields contains every typed descriptor shared by the Demo and real
// integration checks.
type LDAPFields struct {
	RecordID         weaveldap.Attribute[string]
	Number           weaveldap.Attribute[int64]
	Text             weaveldap.Attribute[string]
	NullableNumber   weaveldap.Attribute[int64]
	NullableText     weaveldap.Attribute[string]
	EqualityOnlyText weaveldap.Attribute[string]
	Tags             weaveldap.Attribute[string]
	EmptyIA5         weaveldap.Attribute[string]
	Octets           weaveldap.Attribute[[]byte]
}

// LDAPAdapter contains one immutable Schema, Compiler, Factory, and descriptor
// set. It contains no directory connection or credentials.
type LDAPAdapter struct {
	Schema   weaveldap.Schema
	Compiler weaveldap.Compiler
	Factory  *weaveldap.Factory
	Fields   LDAPFields
}

// NewLDAPAdapter creates the application-controlled descriptor set matching
// the committed OpenLDAP schema.
func NewLDAPAdapter() (LDAPAdapter, error) {
	caseExact, err := weaveldap.NewMatchingRules(
		caseExactMatchOID,
		caseExactOrderingMatchOID,
		caseExactSubstringMatchOID,
	)
	if err != nil {
		return LDAPAdapter{}, fmt.Errorf("create LDAP case-exact matching rules: %w", err)
	}
	integer, err := weaveldap.NewMatchingRules(
		integerMatchOID,
		integerOrderingMatchOID,
		"",
	)
	if err != nil {
		return LDAPAdapter{}, fmt.Errorf("create LDAP integer matching rules: %w", err)
	}
	equalityOnly, err := weaveldap.NewMatchingRules(caseExactMatchOID, "", "")
	if err != nil {
		return LDAPAdapter{}, fmt.Errorf("create LDAP equality matching rules: %w", err)
	}
	ia5, err := weaveldap.NewMatchingRules(caseExactIA5MatchOID, "", "")
	if err != nil {
		return LDAPAdapter{}, fmt.Errorf("create LDAP IA5 matching rules: %w", err)
	}
	octets, err := weaveldap.NewMatchingRules(octetStringMatchOID, "", "")
	if err != nil {
		return LDAPAdapter{}, fmt.Errorf("create LDAP octet matching rules: %w", err)
	}

	textOperators := weave.NewOperatorSet(
		weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorIn,
		weave.OperatorNotIn,
		weave.OperatorNotNull,
		weave.OperatorContains,
		weave.OperatorHasPrefix,
		weave.OperatorHasSuffix,
	)
	numberOperators := weave.NewOperatorSet(
		weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorLTE,
		weave.OperatorGTE,
		weave.OperatorIn,
		weave.OperatorNotIn,
		weave.OperatorBetween,
		weave.OperatorNotNull,
	)
	equalityOperators := weave.NewOperatorSet(
		weave.OperatorEQ,
		weave.OperatorNEQ,
		weave.OperatorIn,
		weave.OperatorNotIn,
		weave.OperatorNotNull,
	)

	recordID, err := newLDAPStringAttribute(
		fixture.LDAPRecordID,
		fixture.LDAPRecordIDOID,
		weaveldap.SyntaxDirectoryString,
		caseExact,
		textOperators,
	)
	if err != nil {
		return LDAPAdapter{}, err
	}
	number, err := newLDAPIntegerAttribute(
		fixture.LDAPNumber,
		fixture.LDAPNumberOID,
		integer,
		numberOperators,
	)
	if err != nil {
		return LDAPAdapter{}, err
	}
	text, err := newLDAPStringAttribute(
		fixture.LDAPText,
		fixture.LDAPTextOID,
		weaveldap.SyntaxDirectoryString,
		caseExact,
		textOperators,
	)
	if err != nil {
		return LDAPAdapter{}, err
	}
	nullableNumber, err := newLDAPIntegerAttribute(
		fixture.LDAPNullableNumber,
		fixture.LDAPNullableNumberOID,
		integer,
		numberOperators,
	)
	if err != nil {
		return LDAPAdapter{}, err
	}
	nullableText, err := newLDAPStringAttribute(
		fixture.LDAPNullableText,
		fixture.LDAPNullableTextOID,
		weaveldap.SyntaxDirectoryString,
		caseExact,
		textOperators,
	)
	if err != nil {
		return LDAPAdapter{}, err
	}
	equalityOnlyText, err := newLDAPStringAttribute(
		fixture.LDAPEqualityOnlyText,
		fixture.LDAPEqualityOnlyTextOID,
		weaveldap.SyntaxDirectoryString,
		equalityOnly,
		equalityOperators,
	)
	if err != nil {
		return LDAPAdapter{}, err
	}
	tags, err := weaveldap.NewAttribute[string](weaveldap.AttributeSpec{
		Description:  fixture.LDAPTags,
		OID:          fixture.LDAPTagsOID,
		SingleValued: false,
		Syntax:       weaveldap.SyntaxDirectoryString,
		Matching:     caseExact,
	})
	if err != nil {
		return LDAPAdapter{}, fmt.Errorf("create LDAP multi-valued descriptor: %w", err)
	}
	emptyIA5, err := newLDAPStringAttribute(
		fixture.LDAPEmptyIA5,
		fixture.LDAPEmptyIA5OID,
		weaveldap.SyntaxIA5String,
		ia5,
		equalityOperators,
	)
	if err != nil {
		return LDAPAdapter{}, err
	}
	octetAttribute, err := weaveldap.NewAttribute[[]byte](weaveldap.AttributeSpec{
		Description:  fixture.LDAPOctets,
		OID:          fixture.LDAPOctetsOID,
		SingleValued: true,
		Syntax:       weaveldap.SyntaxOctetString,
		Matching:     octets,
		Operators:    equalityOperators,
	})
	if err != nil {
		return LDAPAdapter{}, fmt.Errorf("create LDAP octet descriptor: %w", err)
	}

	fields := LDAPFields{
		RecordID:         recordID,
		Number:           number,
		Text:             text,
		NullableNumber:   nullableNumber,
		NullableText:     nullableText,
		EqualityOnlyText: equalityOnlyText,
		Tags:             tags,
		EmptyIA5:         emptyIA5,
		Octets:           octetAttribute,
	}
	schema, err := weaveldap.NewSchema(
		fields.RecordID,
		fields.Number,
		fields.Text,
		fields.NullableNumber,
		fields.NullableText,
		fields.EqualityOnlyText,
		fields.Tags,
		fields.EmptyIA5,
		fields.Octets,
	)
	if err != nil {
		return LDAPAdapter{}, fmt.Errorf("create LDAP Schema: %w", err)
	}
	compiler, err := weaveldap.NewCompiler(weaveldap.RFC4515, schema)
	if err != nil {
		return LDAPAdapter{}, fmt.Errorf("create LDAP Compiler: %w", err)
	}
	factory, err := weaveldap.NewFactory(weaveldap.RFC4515, schema)
	if err != nil {
		return LDAPAdapter{}, fmt.Errorf("create LDAP Factory: %w", err)
	}
	return LDAPAdapter{
		Schema: schema, Compiler: compiler, Factory: factory, Fields: fields,
	}, nil
}

// RunLDAP executes every applicable canonical scenario against the exact
// OpenLDAP server pinned by the directory profile.
func RunLDAP(ctx context.Context) (scenario.Report, error) {
	harness, server, err := NewLDAPHarness(ctx)
	if err != nil {
		return scenario.Report{}, err
	}
	return scenario.Run(
		ctx,
		"ldap",
		"openldap-"+server.Version,
		harness,
	)
}

// NewLDAPHarness validates and resets the live service, then returns a
// compilertest harness whose execution path performs real LDAP searches.
func NewLDAPHarness(
	ctx context.Context,
) (
	compilertest.Harness[weaveldap.Filter, weaveldap.Expression],
	testenv.LDAPServerInfo,
	error,
) {
	var zero compilertest.Harness[weaveldap.Filter, weaveldap.Expression]
	if ctx == nil {
		return zero, testenv.LDAPServerInfo{}, fmt.Errorf("create LDAP harness: nil context")
	}
	adapter, err := NewLDAPAdapter()
	if err != nil {
		return zero, testenv.LDAPServerInfo{}, err
	}
	config, err := testenv.LoadLDAPConfig()
	if err != nil {
		return zero, testenv.LDAPServerInfo{}, err
	}
	connection, err := testenv.WaitForLDAP(ctx, config, 250*time.Millisecond)
	if err != nil {
		return zero, testenv.LDAPServerInfo{}, err
	}
	server, err := testenv.ReadLDAPServerInfo(connection)
	if err == nil {
		err = testenv.ResetLDAP(connection)
	}
	testenv.CloseLDAP(connection)
	if err != nil {
		return zero, testenv.LDAPServerInfo{}, err
	}

	fields := compilertest.Fields{
		Number:           adapter.Fields.Number,
		Text:             adapter.Fields.Text,
		NullableNumber:   adapter.Fields.NullableNumber,
		NullableText:     adapter.Fields.NullableText,
		EqualityOnlyText: adapter.Fields.EqualityOnlyText,
	}
	harness := compilertest.Harness[weaveldap.Filter, weaveldap.Expression]{
		Factory:  adapter.Factory,
		Fields:   fields,
		Resolver: adapter.Compiler,
		Execute: func(filter weaveldap.Filter) ([]string, error) {
			if !filter.Valid() {
				return nil, fmt.Errorf("execute invalid LDAP Filter")
			}
			return testenv.QueryLDAPIDs(
				ctx,
				config,
				fixture.LDAPRecordsDN,
				filter.String(),
			)
		},
		InspectCondition: InspectLDAPCondition,
		NativeCondition: func(ids []string) weaveldap.Filter {
			filter, _ := weaveldap.ParseFilter(
				adapter.Schema,
				ldapIDFilter(ids),
			)
			return filter
		},
		NativeExpression: func(ids []string) weaveldap.Expression {
			return ldapIDFilter(ids)
		},
		NilLikeNativeCondition: func() weaveldap.Filter { return weaveldap.Filter{} },
		DistinguishesMissing:   false,
	}
	return harness, server, nil
}

// InspectLDAPCondition verifies that a package-owned Filter remains accepted
// by the locked go-ldap codec without returning its text in an error.
func InspectLDAPCondition(_ string, filter weaveldap.Filter) error {
	if !filter.Valid() || filter.String() == "" {
		return fmt.Errorf("LDAP condition is invalid")
	}
	packet, err := ldapv3.CompileFilter(filter.String())
	if err != nil || packet == nil {
		return fmt.Errorf("LDAP condition is not codec-valid")
	}
	return nil
}

func newLDAPStringAttribute(
	description string,
	oid string,
	syntax weaveldap.Syntax,
	matching weaveldap.MatchingRules,
	operators weave.OperatorSet,
) (weaveldap.Attribute[string], error) {
	attribute, err := weaveldap.NewAttribute[string](weaveldap.AttributeSpec{
		Description:  description,
		OID:          oid,
		SingleValued: true,
		Syntax:       syntax,
		Matching:     matching,
		Operators:    operators,
	})
	if err != nil {
		return weaveldap.Attribute[string]{}, fmt.Errorf("create LDAP string descriptor: %w", err)
	}
	return attribute, nil
}

func newLDAPIntegerAttribute(
	description string,
	oid string,
	matching weaveldap.MatchingRules,
	operators weave.OperatorSet,
) (weaveldap.Attribute[int64], error) {
	attribute, err := weaveldap.NewAttribute[int64](weaveldap.AttributeSpec{
		Description:  description,
		OID:          oid,
		SingleValued: true,
		Syntax:       weaveldap.SyntaxInteger,
		Matching:     matching,
		Operators:    operators,
	})
	if err != nil {
		return weaveldap.Attribute[int64]{}, fmt.Errorf("create LDAP integer descriptor: %w", err)
	}
	return attribute, nil
}

func ldapIDFilter(ids []string) string {
	children := make([]string, len(ids))
	for index, id := range ids {
		children[index] = "(" + fixture.LDAPRecordID + "=" + ldapv3.EscapeFilter(id) + ")"
	}
	switch len(children) {
	case 0:
		return "(&(" + fixture.LDAPRecordID + "=*)(!(" + fixture.LDAPRecordID + "=*)))"
	case 1:
		return children[0]
	default:
		return "(|" + strings.Join(children, "") + ")"
	}
}
