package main

import (
	"reflect"
	"testing"

	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
)

func TestSelectedServices(t *testing.T) {
	tests := []struct {
		name              string
		value             string
		wantSQL           []testenv.Backend
		wantMongo         bool
		wantLDAP          bool
		wantElasticsearch bool
		wantError         bool
	}{
		{
			name: "all", value: " all ", wantSQL: testenv.SQLBackends(),
			wantMongo: true, wantLDAP: true, wantElasticsearch: true,
		},
		{name: "sql", value: "SQL", wantSQL: testenv.SQLBackends()},
		{name: "mysql", value: "mysql", wantSQL: []testenv.Backend{testenv.MySQL}},
		{name: "mongo", value: "Mongo", wantMongo: true},
		{name: "directory", value: "Directory", wantLDAP: true},
		{name: "ldap", value: "LDAP", wantLDAP: true},
		{name: "search", value: "Search", wantElasticsearch: true},
		{name: "elasticsearch", value: "Elasticsearch", wantElasticsearch: true},
		{name: "invalid", value: "private-invalid", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := selectedServices(test.value)
			if (err != nil) != test.wantError {
				t.Fatalf("selectedServices() error = %v, wantError %v", err, test.wantError)
			}
			if test.wantError {
				return
			}
			if !reflect.DeepEqual(got.sqlBackends, test.wantSQL) ||
				got.mongo != test.wantMongo || got.ldap != test.wantLDAP ||
				got.elasticsearch != test.wantElasticsearch {
				t.Fatalf("selectedServices() = %#v", got)
			}
		})
	}
}
