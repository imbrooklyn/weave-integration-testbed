package fixture

import (
	"slices"
	"testing"

	"github.com/imbrooklyn/weave/compilertest"
)

func TestLDAPEntriesMaterializeCanonicalRecordsAndSeams(t *testing.T) {
	entries := LDAPEntries()
	if got, want := len(entries), len(compilertest.Records())+5; got != want {
		t.Fatalf("LDAPEntries() count = %d, want %d", got, want)
	}
	if got, want := LDAPRecordDNs(), []string{
		"weaveRecordID=r01," + LDAPRecordsDN,
		"weaveRecordID=r02," + LDAPRecordsDN,
		"weaveRecordID=r03," + LDAPRecordsDN,
		"weaveRecordID=r04," + LDAPRecordsDN,
		"weaveRecordID=r05," + LDAPRecordsDN,
		"weaveRecordID=r06," + LDAPRecordsDN,
	}; !slices.Equal(got, want) {
		t.Fatalf("LDAPRecordDNs() = %v, want %v", got, want)
	}

	for _, index := range []int{2, 3} {
		for _, attribute := range entries[index].Attributes {
			if attribute.Name == LDAPNullableNumber || attribute.Name == LDAPNullableText {
				t.Fatalf("entry %d materialized absent nullable attribute", index)
			}
		}
	}

	first := LDAPEntries()
	first[0].Attributes[0].Values[0] = "changed"
	if LDAPEntries()[0].Attributes[0].Values[0] != "top" {
		t.Fatal("LDAPEntries() leaked mutable value storage")
	}
}
