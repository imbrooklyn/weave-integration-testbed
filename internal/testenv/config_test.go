package testenv

import (
	"strings"
	"testing"
)

func TestLoadSQLConfigDefaults(t *testing.T) {
	for _, backend := range SQLBackends() {
		config, err := LoadSQLConfig(backend)
		if err != nil {
			t.Fatalf("LoadSQLConfig(%s) error = %v", backend, err)
		}
		if config.Backend != backend || config.Host != "127.0.0.1" {
			t.Fatalf("LoadSQLConfig(%s) = %#v", backend, config)
		}
		if strings.Contains(config.Endpoint(), config.Password) {
			t.Fatalf("Endpoint() exposes password for %s", backend)
		}
	}
}

func TestLoadSQLConfigRejectsInvalidPortWithoutEchoingValue(t *testing.T) {
	const invalid = "not-a-port-secret"
	t.Setenv("WEAVE_TESTBED_MYSQL_PORT", invalid)
	_, err := LoadSQLConfig(MySQL)
	if err == nil {
		t.Fatal("LoadSQLConfig(MySQL) error = nil")
	}
	if strings.Contains(err.Error(), invalid) {
		t.Fatalf("LoadSQLConfig(MySQL) error exposes the invalid value: %v", err)
	}
}

func TestParseBackend(t *testing.T) {
	backend, err := ParseBackend(" PostgreS ")
	if err != nil || backend != PostgreSQL {
		t.Fatalf("ParseBackend() = (%q, %v)", backend, err)
	}
	if _, err := ParseBackend("unknown"); err == nil {
		t.Fatal("ParseBackend(unknown) error = nil")
	}
}
