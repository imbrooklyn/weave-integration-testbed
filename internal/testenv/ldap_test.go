package testenv

import (
	"strings"
	"testing"
)

func TestLDAPConfigEndpointOmitsCredentials(t *testing.T) {
	config := LDAPConfig{
		Host:          "127.0.0.1",
		Port:          defaultLDAPPort,
		BaseDN:        defaultLDAPBaseDN,
		AdminUser:     "endpoint-private-user",
		AdminPassword: "endpoint-private-password",
	}
	endpoint := config.Endpoint()
	if !strings.Contains(endpoint, "ldap://") ||
		strings.Contains(endpoint, config.AdminPassword) ||
		strings.Contains(endpoint, config.AdminUser) {
		t.Fatalf("LDAP endpoint is not credential-free: %q", endpoint)
	}
}

func TestLDAPConfigRejectsForeignFixtureSuffix(t *testing.T) {
	config, err := LoadLDAPConfig()
	if err != nil {
		t.Fatal(err)
	}
	config.BaseDN = "dc=foreign,dc=test"
	if err := config.validate(); err == nil {
		t.Fatal("LDAPConfig accepted a foreign fixture suffix")
	}
}
