// Package testenv configures and controls the local integration services.
package testenv

import (
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Backend identifies one supported SQL service.
type Backend string

const (
	// MySQL identifies the MySQL service in the sql profile.
	MySQL Backend = "mysql"
	// PostgreSQL identifies the PostgreSQL service in the sql profile.
	PostgreSQL Backend = "postgres"
)

type environmentDefaults struct {
	prefix   string
	host     string
	port     uint16
	database string
	user     string
	password string
}

var defaultsByBackend = map[Backend]environmentDefaults{
	MySQL: {
		prefix:   "WEAVE_TESTBED_MYSQL",
		host:     "127.0.0.1",
		port:     33306,
		database: "weave_testbed",
		user:     "weave",
		password: "weave_demo_only",
	},
	PostgreSQL: {
		prefix:   "WEAVE_TESTBED_POSTGRES",
		host:     "127.0.0.1",
		port:     35432,
		database: "weave_testbed",
		user:     "weave",
		password: "weave_demo_only",
	},
}

// SQLConfig contains the non-secret service identity and credentials loaded
// from the documented environment variables. Callers must not log Password.
type SQLConfig struct {
	Backend  Backend
	Host     string
	Port     uint16
	Database string
	User     string
	Password string
}

// SQLBackends returns the supported backends in stable display order.
func SQLBackends() []Backend {
	return []Backend{MySQL, PostgreSQL}
}

// ParseBackend validates a command-line backend name.
func ParseBackend(value string) (Backend, error) {
	backend := Backend(strings.ToLower(strings.TrimSpace(value)))
	if _, ok := defaultsByBackend[backend]; !ok {
		return "", fmt.Errorf("unsupported SQL backend %q", value)
	}
	return backend, nil
}

// LoadSQLConfig reads one backend configuration from the environment and
// applies the same local-only defaults as compose.yaml.
func LoadSQLConfig(backend Backend) (SQLConfig, error) {
	defaults, ok := defaultsByBackend[backend]
	if !ok {
		return SQLConfig{}, fmt.Errorf("unsupported SQL backend %q", backend)
	}
	port, err := environmentPort(defaults.prefix+"_PORT", defaults.port)
	if err != nil {
		return SQLConfig{}, err
	}
	config := SQLConfig{
		Backend:  backend,
		Host:     environmentValue(defaults.prefix+"_HOST", defaults.host),
		Port:     port,
		Database: environmentValue(defaults.prefix+"_DATABASE", defaults.database),
		User:     environmentValue(defaults.prefix+"_USER", defaults.user),
		Password: environmentValue(defaults.prefix+"_PASSWORD", defaults.password),
	}
	if err := config.validate(); err != nil {
		return SQLConfig{}, err
	}
	return config, nil
}

// Endpoint returns a credential-free service description suitable for logs.
func (config SQLConfig) Endpoint() string {
	return fmt.Sprintf(
		"%s://%s/%s",
		config.Backend,
		net.JoinHostPort(config.Host, strconv.Itoa(int(config.Port))),
		config.Database,
	)
}

// OpenSQL creates a database/sql handle with the public MySQL or pgx driver
// API. It does not contact the service; use WaitForSQL for readiness.
func OpenSQL(config SQLConfig) (*sql.DB, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	var (
		database *sql.DB
		err      error
	)
	switch config.Backend {
	case MySQL:
		driverConfig := mysql.NewConfig()
		driverConfig.User = config.User
		driverConfig.Passwd = config.Password
		driverConfig.Net = "tcp"
		driverConfig.Addr = net.JoinHostPort(
			config.Host,
			strconv.Itoa(int(config.Port)),
		)
		driverConfig.DBName = config.Database
		driverConfig.Collation = "utf8mb4_bin"
		driverConfig.ParseTime = true
		driverConfig.Loc = time.UTC
		driverConfig.Timeout = 5 * time.Second
		driverConfig.ReadTimeout = 5 * time.Second
		driverConfig.WriteTimeout = 5 * time.Second
		database, err = sql.Open("mysql", driverConfig.FormatDSN())
	case PostgreSQL:
		connectionURL := &url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(config.User, config.Password),
			Host: net.JoinHostPort(
				config.Host,
				strconv.Itoa(int(config.Port)),
			),
			Path: "/" + config.Database,
		}
		parameters := connectionURL.Query()
		parameters.Set("connect_timeout", "5")
		parameters.Set("sslmode", "disable")
		parameters.Set("timezone", "UTC")
		connectionURL.RawQuery = parameters.Encode()
		pgxConfig, parseErr := pgx.ParseConfig(connectionURL.String())
		if parseErr != nil {
			return nil, fmt.Errorf("build PostgreSQL connection configuration")
		}
		database = stdlib.OpenDB(*pgxConfig)
	default:
		return nil, fmt.Errorf("unsupported SQL backend %q", config.Backend)
	}
	if err != nil {
		return nil, fmt.Errorf("open %s database handle: %w", config.Backend, err)
	}
	database.SetMaxOpenConns(16)
	database.SetMaxIdleConns(4)
	database.SetConnMaxLifetime(5 * time.Minute)
	return database, nil
}

func (config SQLConfig) validate() error {
	if _, ok := defaultsByBackend[config.Backend]; !ok {
		return fmt.Errorf("unsupported SQL backend %q", config.Backend)
	}
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "host", value: config.Host},
		{name: "database", value: config.Database},
		{name: "user", value: config.User},
		{name: "password", value: config.Password},
	} {
		if strings.TrimSpace(item.value) == "" {
			return fmt.Errorf("%s configuration is empty", item.name)
		}
	}
	if config.Port == 0 {
		return fmt.Errorf("port configuration is invalid")
	}
	return nil
}

func environmentValue(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func environmentPort(name string, fallback uint16) (uint16, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil || port == 0 {
		return 0, fmt.Errorf("%s must be an integer from 1 through 65535", name)
	}
	return uint16(port), nil
}
