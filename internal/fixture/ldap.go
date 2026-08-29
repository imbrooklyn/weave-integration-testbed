package fixture

import (
	"fmt"
	"strconv"

	"github.com/imbrooklyn/weave/compilertest"
)

const (
	// LDAPBaseDN is the suffix initialized by the directory Compose profile.
	LDAPBaseDN = "dc=weave,dc=test"
	// LDAPRecordsDN contains the canonical compilertest records.
	LDAPRecordsDN = "ou=records," + LDAPBaseDN
	// LDAPProbesDN contains LDAP-only escaping and cardinality seam records.
	LDAPProbesDN = "ou=probes," + LDAPBaseDN

	// LDAPRecordID identifies the stable record-ID attribute.
	LDAPRecordID = "weaveRecordID"
	// LDAPNumber identifies the always-present integer attribute.
	LDAPNumber = "weaveNumber"
	// LDAPText identifies the always-present text attribute.
	LDAPText = "weaveText"
	// LDAPNullableNumber identifies the optional integer attribute.
	LDAPNullableNumber = "weaveNullableNumber"
	// LDAPNullableText identifies the optional text attribute.
	LDAPNullableText = "weaveNullableText"
	// LDAPEqualityOnlyText identifies the equality-only text attribute.
	LDAPEqualityOnlyText = "weaveEqualityOnlyText"
	// LDAPTags identifies the multi-valued text attribute.
	LDAPTags = "weaveTags"
	// LDAPEmptyIA5 identifies the optional empty-capable IA5 attribute.
	LDAPEmptyIA5 = "weaveEmptyIA5"
	// LDAPOctets identifies the optional arbitrary-octet attribute.
	LDAPOctets = "weaveOctets"

	// LDAPRecordIDOID is the schema OID emitted for LDAPRecordID.
	LDAPRecordIDOID = "1.3.6.1.4.1.55555.13.1.1"
	// LDAPNumberOID is the schema OID emitted for LDAPNumber.
	LDAPNumberOID = "1.3.6.1.4.1.55555.13.1.2"
	// LDAPTextOID is the schema OID emitted for LDAPText.
	LDAPTextOID = "1.3.6.1.4.1.55555.13.1.3"
	// LDAPNullableNumberOID is the schema OID emitted for LDAPNullableNumber.
	LDAPNullableNumberOID = "1.3.6.1.4.1.55555.13.1.4"
	// LDAPNullableTextOID is the schema OID emitted for LDAPNullableText.
	LDAPNullableTextOID = "1.3.6.1.4.1.55555.13.1.5"
	// LDAPEqualityOnlyTextOID is the schema OID emitted for LDAPEqualityOnlyText.
	LDAPEqualityOnlyTextOID = "1.3.6.1.4.1.55555.13.1.6"
	// LDAPTagsOID is the schema OID emitted for LDAPTags.
	LDAPTagsOID = "1.3.6.1.4.1.55555.13.1.7"
	// LDAPEmptyIA5OID is the schema OID emitted for LDAPEmptyIA5.
	LDAPEmptyIA5OID = "1.3.6.1.4.1.55555.13.1.8"
	// LDAPOctetsOID is the schema OID emitted for LDAPOctets.
	LDAPOctetsOID = "1.3.6.1.4.1.55555.13.1.9"

	// LDAPSpecialText contains literal RFC 4515 delimiters and Unicode.
	LDAPSpecialText = "literal *()\\ 世界"
	// LDAPInjectionText resembles a complete caller-controlled filter fragment.
	LDAPInjectionText = "*)(|(weaveRecordID=*))"
	// LDAPNULOctets includes a NUL byte inside an assertion value.
	LDAPNULOctets = "head\x00tail"
)

// LDAPAttribute is one deterministic attribute in a fixture add request.
type LDAPAttribute struct {
	Name   string
	Values []string
}

// LDAPEntry is one deterministic directory entry. LDAPEntries returns fresh
// value slices, so callers may prepare driver requests without sharing state.
type LDAPEntry struct {
	DN         string
	Attributes []LDAPAttribute
}

// LDAPEntries materializes the canonical compilertest records and LDAP-only
// seam probes. Explicit null and missing canonical values are both represented
// by an absent LDAP attribute because LDAP has no portable explicit NULL.
func LDAPEntries() []LDAPEntry {
	records := compilertest.Records()
	entries := make([]LDAPEntry, 0, len(records)+5)
	for _, record := range records {
		attributes := requiredLDAPAttributes(record.ID, record.Number, record.Text)
		if record.NullableNumber != nil {
			attributes = append(attributes, LDAPAttribute{
				Name:   LDAPNullableNumber,
				Values: []string{strconv.FormatInt(*record.NullableNumber, 10)},
			})
		}
		if record.NullableText != nil {
			attributes = append(attributes, LDAPAttribute{
				Name:   LDAPNullableText,
				Values: []string{*record.NullableText},
			})
		}
		switch record.ID {
		case "r01":
			attributes = append(attributes, LDAPAttribute{
				Name: LDAPTags, Values: []string{"red", "shared"},
			})
		case "r02":
			attributes = append(attributes, LDAPAttribute{
				Name: LDAPTags, Values: []string{"green", "shared"},
			})
		}
		entries = append(entries, LDAPEntry{
			DN:         recordDN(record.ID, LDAPRecordsDN),
			Attributes: attributes,
		})
	}

	entries = append(entries,
		probeEntry("p01", 101, LDAPSpecialText,
			LDAPAttribute{Name: LDAPTags, Values: []string{"one", "two"}},
		),
		probeEntry("p02", 102, LDAPInjectionText),
		probeEntry("p03", 103, "embedded NUL octets",
			LDAPAttribute{Name: LDAPOctets, Values: []string{LDAPNULOctets}},
		),
		probeEntry("p04", 104, "present empty IA5 value",
			LDAPAttribute{Name: LDAPEmptyIA5, Values: []string{""}},
		),
		probeEntry("p05", 105, "absent empty IA5 attribute"),
	)
	return entries
}

// LDAPRecordDNs returns the stable canonical record DNs in fixture order.
func LDAPRecordDNs() []string {
	records := compilertest.Records()
	dns := make([]string, len(records))
	for index, record := range records {
		dns[index] = recordDN(record.ID, LDAPRecordsDN)
	}
	return dns
}

func requiredLDAPAttributes(id string, number int64, text string) []LDAPAttribute {
	return []LDAPAttribute{
		{Name: "objectClass", Values: []string{"top", "weaveRecord"}},
		{Name: LDAPRecordID, Values: []string{id}},
		{Name: LDAPNumber, Values: []string{strconv.FormatInt(number, 10)}},
		{Name: LDAPText, Values: []string{text}},
		{Name: LDAPEqualityOnlyText, Values: []string{text}},
	}
}

func probeEntry(id string, number int64, text string, extra ...LDAPAttribute) LDAPEntry {
	attributes := requiredLDAPAttributes(id, number, text)
	attributes = append(attributes, extra...)
	return LDAPEntry{
		DN:         recordDN(id, LDAPProbesDN),
		Attributes: attributes,
	}
}

func recordDN(id, parent string) string {
	return fmt.Sprintf("%s=%s,%s", LDAPRecordID, id, parent)
}
