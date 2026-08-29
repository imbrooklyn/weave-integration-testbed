package testenv

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	ldapv3 "github.com/go-ldap/ldap/v3"
	"github.com/imbrooklyn/weave-integration-testbed/internal/fixture"
)

const (
	defaultLDAPHost          = "127.0.0.1"
	defaultLDAPPort          = uint16(3389)
	defaultLDAPBaseDN        = fixture.LDAPBaseDN
	defaultLDAPAdminUser     = "admin"
	defaultLDAPAdminPassword = "weave_demo_only"
	// OpenLDAPVersion is the exact server version pinned by compose.yaml.
	OpenLDAPVersion = "2.6.10"
)

// LDAPConfig contains the directory endpoint and local test credentials.
// Callers must not log AdminPassword or a complete Config value.
type LDAPConfig struct {
	Host          string
	Port          uint16
	BaseDN        string
	AdminUser     string
	AdminPassword string
}

// LDAPServerInfo is the non-secret identity of the pinned directory profile
// after live LDAPv3 and custom-schema verification. The OpenLDAP root DSE does
// not expose its product version; the profile's container binary is verified
// separately by the integration workflow.
type LDAPServerInfo struct {
	Vendor  string
	Version string
}

// LoadLDAPConfig reads directory configuration from the documented
// environment variables and applies the same local-only defaults as Compose.
func LoadLDAPConfig() (LDAPConfig, error) {
	port, err := environmentPort("WEAVE_TESTBED_LDAP_PORT", defaultLDAPPort)
	if err != nil {
		return LDAPConfig{}, err
	}
	config := LDAPConfig{
		Host:          environmentValue("WEAVE_TESTBED_LDAP_HOST", defaultLDAPHost),
		Port:          port,
		BaseDN:        environmentValue("WEAVE_TESTBED_LDAP_BASE_DN", defaultLDAPBaseDN),
		AdminUser:     environmentValue("WEAVE_TESTBED_LDAP_ADMIN_USER", defaultLDAPAdminUser),
		AdminPassword: environmentValue("WEAVE_TESTBED_LDAP_ADMIN_PASSWORD", defaultLDAPAdminPassword),
	}
	if err := config.validate(); err != nil {
		return LDAPConfig{}, err
	}
	return config, nil
}

// Endpoint returns a credential-free directory description suitable for logs.
func (config LDAPConfig) Endpoint() string {
	return fmt.Sprintf(
		"ldap://%s/%s",
		net.JoinHostPort(config.Host, strconv.Itoa(int(config.Port))),
		config.BaseDN,
	)
}

// BindDN returns the local test administrator DN. Do not use it as a log
// identity in code that accepts externally supplied configuration.
func (config LDAPConfig) BindDN() string {
	return "cn=" + config.AdminUser + "," + config.BaseDN
}

// OpenLDAP establishes and binds one finite-timeout connection. Driver errors
// are deliberately omitted so credentials and connection details cannot leak.
func OpenLDAP(config LDAPConfig) (*ldapv3.Conn, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	endpoint := "ldap://" + net.JoinHostPort(
		config.Host,
		strconv.Itoa(int(config.Port)),
	)
	connection, err := ldapv3.DialURL(
		endpoint,
		ldapv3.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}),
	)
	if err != nil {
		return nil, fmt.Errorf("open LDAP connection")
	}
	connection.SetTimeout(5 * time.Second)
	if err := connection.Bind(config.BindDN(), config.AdminPassword); err != nil {
		connection.Close()
		return nil, fmt.Errorf("bind LDAP test administrator")
	}
	return connection, nil
}

// WaitForLDAP retries an authenticated connection and base search until the
// service is ready or ctx expires.
func WaitForLDAP(
	ctx context.Context,
	config LDAPConfig,
	interval time.Duration,
) (*ldapv3.Conn, error) {
	if ctx == nil {
		return nil, fmt.Errorf("wait for LDAP: nil context")
	}
	if interval <= 0 {
		return nil, fmt.Errorf("wait for LDAP: interval must be positive")
	}
	for {
		connection, err := OpenLDAP(config)
		if err == nil {
			request := ldapv3.NewSearchRequest(
				config.BaseDN,
				ldapv3.ScopeBaseObject,
				ldapv3.NeverDerefAliases,
				1,
				5,
				false,
				"(objectClass=*)",
				[]string{"dn"},
				nil,
			)
			if result, searchErr := connection.Search(request); searchErr == nil &&
				len(result.Entries) == 1 {
				return connection, nil
			}
			connection.Close()
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("LDAP did not become healthy: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// ReadLDAPServerInfo validates the live LDAPv3 root DSE and custom schema OIDs.
// The returned product identity is the exact image profile pinned by Compose;
// OpenLDAP 2.6.10 does not publish vendorName or vendorVersion in its root DSE.
func ReadLDAPServerInfo(connection *ldapv3.Conn) (LDAPServerInfo, error) {
	if connection == nil {
		return LDAPServerInfo{}, fmt.Errorf("read LDAP server info: nil connection")
	}
	request := ldapv3.NewSearchRequest(
		"",
		ldapv3.ScopeBaseObject,
		ldapv3.NeverDerefAliases,
		1,
		5,
		false,
		"(objectClass=*)",
		[]string{
			"vendorName", "vendorVersion", "supportedLDAPVersion",
			"subschemaSubentry",
		},
		nil,
	)
	result, err := connection.Search(request)
	if err != nil || len(result.Entries) != 1 {
		return LDAPServerInfo{}, fmt.Errorf("read LDAP root DSE")
	}
	entry := result.Entries[0]
	if !slices.Contains(entry.GetAttributeValues("supportedLDAPVersion"), "3") {
		return LDAPServerInfo{}, fmt.Errorf("LDAP server does not publish LDAPv3 support")
	}
	subschema := entry.GetAttributeValue("subschemaSubentry")
	if strings.TrimSpace(subschema) == "" {
		return LDAPServerInfo{}, fmt.Errorf("LDAP root DSE omits subschemaSubentry")
	}
	if err := verifyLDAPSchema(connection, subschema); err != nil {
		return LDAPServerInfo{}, err
	}
	return LDAPServerInfo{Vendor: "OpenLDAP", Version: OpenLDAPVersion}, nil
}

// ResetLDAP replaces every testbed-owned record and probe entry with fresh
// values derived from fixture.LDAPEntries.
func ResetLDAP(connection *ldapv3.Conn) error {
	if connection == nil {
		return fmt.Errorf("reset LDAP fixture: nil connection")
	}
	for _, parent := range []string{fixture.LDAPRecordsDN, fixture.LDAPProbesDN} {
		if err := deleteLDAPChildren(connection, parent); err != nil {
			return err
		}
	}
	for _, entry := range fixture.LDAPEntries() {
		request := ldapv3.NewAddRequest(entry.DN, nil)
		for _, attribute := range entry.Attributes {
			request.Attribute(attribute.Name, attribute.Values)
		}
		if err := connection.Add(request); err != nil {
			return fmt.Errorf("insert LDAP fixture entry")
		}
	}
	return nil
}

// QueryLDAPIDs executes a filter below baseDN and returns stable record IDs in
// sorted order. Errors never include the filter, values, bind identity, or
// credentials.
func QueryLDAPIDs(
	ctx context.Context,
	config LDAPConfig,
	baseDN string,
	filter string,
) ([]string, error) {
	entries, err := queryLDAP(ctx, config, baseDN, filter)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(entries))
	for index, entry := range entries {
		values := entry.GetAttributeValues(fixture.LDAPRecordID)
		if len(values) != 1 || values[0] == "" {
			return nil, fmt.Errorf("decode LDAP fixture record ID")
		}
		ids[index] = values[0]
	}
	slices.Sort(ids)
	return ids, nil
}

// QueryLDAPDNs executes a filter below baseDN and returns sorted entry DNs.
func QueryLDAPDNs(
	ctx context.Context,
	config LDAPConfig,
	baseDN string,
	filter string,
) ([]string, error) {
	entries, err := queryLDAP(ctx, config, baseDN, filter)
	if err != nil {
		return nil, err
	}
	dns := make([]string, len(entries))
	for index, entry := range entries {
		if entry.DN == "" {
			return nil, fmt.Errorf("decode LDAP fixture DN")
		}
		dns[index] = entry.DN
	}
	slices.Sort(dns)
	return dns, nil
}

// CloseLDAP closes a connection and accepts nil for cleanup paths.
func CloseLDAP(connection *ldapv3.Conn) {
	if connection != nil {
		connection.Close()
	}
}

func queryLDAP(
	ctx context.Context,
	config LDAPConfig,
	baseDN string,
	filter string,
) ([]*ldapv3.Entry, error) {
	if ctx == nil {
		return nil, fmt.Errorf("query LDAP fixture: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("query LDAP fixture: %w", err)
	}
	if baseDN != fixture.LDAPRecordsDN && baseDN != fixture.LDAPProbesDN || filter == "" {
		return nil, fmt.Errorf("query LDAP fixture: invalid request")
	}
	connection, err := OpenLDAP(config)
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	request := ldapv3.NewSearchRequest(
		baseDN,
		ldapv3.ScopeSingleLevel,
		ldapv3.NeverDerefAliases,
		0,
		5,
		false,
		filter,
		[]string{fixture.LDAPRecordID},
		nil,
	)
	result, err := connection.Search(request)
	if err != nil {
		return nil, fmt.Errorf("execute LDAP fixture search")
	}
	return result.Entries, nil
}

func deleteLDAPChildren(connection *ldapv3.Conn, parent string) error {
	request := ldapv3.NewSearchRequest(
		parent,
		ldapv3.ScopeSingleLevel,
		ldapv3.NeverDerefAliases,
		0,
		5,
		true,
		"(objectClass=*)",
		[]string{"1.1"},
		nil,
	)
	result, err := connection.Search(request)
	if err != nil {
		return fmt.Errorf("list LDAP fixture entries")
	}
	for _, entry := range result.Entries {
		if err := connection.Del(ldapv3.NewDelRequest(entry.DN, nil)); err != nil {
			return fmt.Errorf("delete LDAP fixture entry")
		}
	}
	return nil
}

func verifyLDAPSchema(connection *ldapv3.Conn, subschema string) error {
	request := ldapv3.NewSearchRequest(
		subschema,
		ldapv3.ScopeBaseObject,
		ldapv3.NeverDerefAliases,
		1,
		5,
		false,
		"(objectClass=subschema)",
		[]string{"attributeTypes"},
		nil,
	)
	result, err := connection.Search(request)
	if err != nil || len(result.Entries) != 1 {
		return fmt.Errorf("read LDAP subschema")
	}
	joined := strings.Join(result.Entries[0].GetAttributeValues("attributeTypes"), "\n")
	for _, oid := range []string{
		fixture.LDAPRecordIDOID,
		fixture.LDAPNumberOID,
		fixture.LDAPTextOID,
		fixture.LDAPNullableNumberOID,
		fixture.LDAPNullableTextOID,
		fixture.LDAPEqualityOnlyTextOID,
		fixture.LDAPTagsOID,
		fixture.LDAPEmptyIA5OID,
		fixture.LDAPOctetsOID,
	} {
		if !strings.Contains(joined, oid) {
			return fmt.Errorf("LDAP server does not expose the required custom schema")
		}
	}
	return nil
}

func (config LDAPConfig) validate() error {
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "LDAP host", value: config.Host},
		{name: "LDAP base DN", value: config.BaseDN},
		{name: "LDAP administrator", value: config.AdminUser},
		{name: "LDAP administrator password", value: config.AdminPassword},
	} {
		if strings.TrimSpace(item.value) == "" {
			return fmt.Errorf("%s configuration is empty", item.name)
		}
	}
	if config.Port == 0 {
		return fmt.Errorf("LDAP port configuration is invalid")
	}
	if config.BaseDN != fixture.LDAPBaseDN {
		return fmt.Errorf("LDAP base DN does not match the committed fixture")
	}
	return nil
}
