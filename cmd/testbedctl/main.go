// Command testbedctl controls the local test service fixtures.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	goelasticsearch "github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/imbrooklyn/weave-integration-testbed/internal/fixture"
	"github.com/imbrooklyn/weave-integration-testbed/internal/testenv"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
)

const defaultTimeout = 2 * time.Minute

type options struct {
	backend string
	root    string
	timeout time.Duration
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "testbedctl: %v\n", err)
		os.Exit(1)
	}
}

func run(arguments []string, output io.Writer) error {
	if len(arguments) == 0 {
		return fmt.Errorf("expected one command: health, reset, verify, or check")
	}
	command := arguments[0]
	switch command {
	case "health", "reset", "verify", "check":
	default:
		return fmt.Errorf("unsupported command %q", command)
	}

	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	config := options{}
	flags.StringVar(
		&config.backend,
		"backend",
		"all",
		"service selection: all, sql, mysql, postgres, mongo, directory, ldap, search, or elasticsearch",
	)
	flags.StringVar(&config.root, "root", ".", "repository root containing testdata")
	flags.DurationVar(&config.timeout, "timeout", defaultTimeout, "timeout per service")
	if err := flags.Parse(arguments[1:]); err != nil {
		return fmt.Errorf("parse %s options: %w", command, err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments for %s", command)
	}
	if config.timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	selection, err := selectedServices(config.backend)
	if err != nil {
		return err
	}

	recordsByBackend := make(
		map[testenv.Backend][]fixture.SQLRecord,
		len(selection.sqlBackends),
	)
	var failures []error
	for _, backend := range selection.sqlBackends {
		records, runErr := runForBackend(command, config, backend, output)
		if runErr != nil {
			failures = append(failures, runErr)
			continue
		}
		if records != nil {
			recordsByBackend[backend] = records
		}
	}
	if selection.mongo {
		if runErr := runForMongo(command, config, output); runErr != nil {
			failures = append(failures, runErr)
		}
	}
	if selection.ldap {
		if runErr := runForLDAP(command, config, output); runErr != nil {
			failures = append(failures, runErr)
		}
	}
	if selection.elasticsearch {
		if runErr := runForElasticsearch(command, config, output); runErr != nil {
			failures = append(failures, runErr)
		}
	}
	if len(failures) != 0 {
		return errors.Join(failures...)
	}
	if command == "check" && len(selection.sqlBackends) == 2 {
		if err := fixture.CompareSQLRecords(
			recordsByBackend[testenv.MySQL],
			recordsByBackend[testenv.PostgreSQL],
		); err != nil {
			return fmt.Errorf("compare SQL fixtures: %w", err)
		}
		fmt.Fprintln(output, "sql: MySQL and PostgreSQL fixtures match")
	}
	return nil
}

func runForElasticsearch(
	command string,
	options options,
	output io.Writer,
) error {
	config, err := testenv.LoadElasticsearchConfig()
	if err != nil {
		return fmt.Errorf("load Elasticsearch configuration: %w", err)
	}
	client, err := testenv.OpenElasticsearch(config)
	if err != nil {
		return err
	}
	defer testenv.CloseElasticsearch(client)
	ctx, cancel := context.WithTimeout(context.Background(), options.timeout)
	defer cancel()
	if err := testenv.WaitForElasticsearch(ctx, client, 500*time.Millisecond); err != nil {
		return err
	}
	server, err := testenv.ReadElasticsearchServerInfo(ctx, client)
	if err != nil {
		return err
	}

	switch command {
	case "health":
		fmt.Fprintf(
			output,
			"elasticsearch: healthy (Elasticsearch %s, Lucene %s)\n",
			server.Version,
			server.LuceneVersion,
		)
		return nil
	case "reset":
		if err := testenv.ResetElasticsearch(ctx, client, config, options.root); err != nil {
			return err
		}
		fmt.Fprintln(output, "elasticsearch: fixture reset")
		return nil
	case "verify":
		count, err := verifyElasticsearchFixture(ctx, client, config)
		if err != nil {
			return err
		}
		fmt.Fprintf(
			output,
			"elasticsearch: fixture IDs verified (%d records, Elasticsearch %s, Lucene %s)\n",
			count,
			server.Version,
			server.LuceneVersion,
		)
		return nil
	case "check":
		if err := testenv.ResetElasticsearch(ctx, client, config, options.root); err != nil {
			return err
		}
		count, err := verifyElasticsearchFixture(ctx, client, config)
		if err != nil {
			return err
		}
		fmt.Fprintf(
			output,
			"elasticsearch: fixture reset and verified (%d records, Elasticsearch %s, Lucene %s)\n",
			count,
			server.Version,
			server.LuceneVersion,
		)
		return nil
	default:
		return fmt.Errorf("unsupported command %q", command)
	}
}

func runForLDAP(
	command string,
	options options,
	output io.Writer,
) error {
	config, err := testenv.LoadLDAPConfig()
	if err != nil {
		return fmt.Errorf("load LDAP configuration: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), options.timeout)
	defer cancel()
	connection, err := testenv.WaitForLDAP(ctx, config, 500*time.Millisecond)
	if err != nil {
		return err
	}
	defer testenv.CloseLDAP(connection)
	server, err := testenv.ReadLDAPServerInfo(connection)
	if err != nil {
		return err
	}

	switch command {
	case "health":
		fmt.Fprintf(output, "ldap: healthy (OpenLDAP %s)\n", server.Version)
		return nil
	case "reset":
		if err := testenv.ResetLDAP(connection); err != nil {
			return err
		}
		fmt.Fprintln(output, "ldap: fixture reset")
		return nil
	case "verify":
		count, err := verifyLDAPFixture(ctx, config)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "ldap: fixture IDs verified (%d records, OpenLDAP %s)\n", count, server.Version)
		return nil
	case "check":
		if err := testenv.ResetLDAP(connection); err != nil {
			return err
		}
		count, err := verifyLDAPFixture(ctx, config)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "ldap: fixture reset and verified (%d records, OpenLDAP %s)\n", count, server.Version)
		return nil
	default:
		return fmt.Errorf("unsupported command %q", command)
	}
}

func runForMongo(
	command string,
	options options,
	output io.Writer,
) (resultErr error) {
	config, err := testenv.LoadMongoConfig()
	if err != nil {
		return fmt.Errorf("load MongoDB configuration: %w", err)
	}
	client, err := testenv.OpenMongo(config)
	if err != nil {
		return err
	}
	defer func() {
		if err := testenv.CloseMongo(client); err != nil && resultErr == nil {
			resultErr = err
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), options.timeout)
	defer cancel()
	if err := testenv.WaitForMongo(ctx, client, 500*time.Millisecond); err != nil {
		return err
	}
	server, err := testenv.ReadMongoServerInfo(ctx, client)
	if err != nil {
		return err
	}
	database := client.Database(config.Database)

	switch command {
	case "health":
		fmt.Fprintf(output, "mongo: healthy (MongoDB %s)\n", server.Version)
		return nil
	case "reset":
		if err := testenv.ResetMongo(ctx, database); err != nil {
			return err
		}
		fmt.Fprintln(output, "mongo: fixture reset")
		return nil
	case "verify":
		count, err := verifyMongoFixture(ctx, database.Collection(fixture.MongoCollection))
		if err != nil {
			return fmt.Errorf("verify MongoDB fixture: %w", err)
		}
		fmt.Fprintf(output, "mongo: fixture IDs verified (%d records, MongoDB %s)\n", count, server.Version)
		return nil
	case "check":
		if err := testenv.ResetMongo(ctx, database); err != nil {
			return err
		}
		count, err := verifyMongoFixture(ctx, database.Collection(fixture.MongoCollection))
		if err != nil {
			return fmt.Errorf("verify MongoDB fixture: %w", err)
		}
		fmt.Fprintf(
			output,
			"mongo: fixture reset and verified (%d records, MongoDB %s)\n",
			count,
			server.Version,
		)
		return nil
	default:
		return fmt.Errorf("unsupported command %q", command)
	}
}

func runForBackend(
	command string,
	options options,
	backend testenv.Backend,
	output io.Writer,
) (records []fixture.SQLRecord, resultErr error) {
	config, err := testenv.LoadSQLConfig(backend)
	if err != nil {
		return nil, fmt.Errorf("load %s configuration: %w", backend, err)
	}
	database, err := testenv.OpenSQL(config)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", backend, err)
	}
	defer func() {
		if err := database.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close %s database: %w", backend, err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), options.timeout)
	defer cancel()
	if err := testenv.WaitForSQL(ctx, backend, database, 500*time.Millisecond); err != nil {
		return nil, err
	}

	switch command {
	case "health":
		fmt.Fprintf(output, "%s: healthy\n", backend)
		return nil, nil
	case "reset":
		if err := testenv.ResetSQL(ctx, database, options.root, backend); err != nil {
			return nil, err
		}
		fmt.Fprintf(output, "%s: fixture reset\n", backend)
		return nil, nil
	case "verify":
		records, err = verifyFixture(ctx, database)
		if err != nil {
			return nil, fmt.Errorf("verify %s fixture: %w", backend, err)
		}
		fmt.Fprintf(output, "%s: fixture IDs verified (%d records)\n", backend, len(records))
		return records, nil
	case "check":
		if err := testenv.ResetSQL(ctx, database, options.root, backend); err != nil {
			return nil, err
		}
		records, err = verifyFixture(ctx, database)
		if err != nil {
			return nil, fmt.Errorf("verify %s fixture: %w", backend, err)
		}
		fmt.Fprintf(output, "%s: fixture reset and verified (%d records)\n", backend, len(records))
		return records, nil
	default:
		return nil, fmt.Errorf("unsupported command %q", command)
	}
}

func verifyFixture(
	ctx context.Context,
	database *sql.DB,
) ([]fixture.SQLRecord, error) {
	records, err := testenv.QuerySQLRecords(ctx, database)
	if err != nil {
		return nil, err
	}
	if err := fixture.CompareIDs(fixture.IDs(records), fixture.StableIDs()); err != nil {
		return nil, err
	}
	return records, nil
}

func verifyMongoFixture(
	ctx context.Context,
	collection *mongodriver.Collection,
) (int, error) {
	ids, err := testenv.QueryMongoIDs(ctx, collection, bson.D{})
	if err != nil {
		return 0, err
	}
	if err := fixture.CompareIDs(ids, fixture.StableIDs()); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func verifyLDAPFixture(
	ctx context.Context,
	config testenv.LDAPConfig,
) (int, error) {
	ids, err := testenv.QueryLDAPIDs(
		ctx,
		config,
		fixture.LDAPRecordsDN,
		"(objectClass=weaveRecord)",
	)
	if err != nil {
		return 0, fmt.Errorf("verify LDAP fixture")
	}
	if err := fixture.CompareIDs(ids, fixture.StableIDs()); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func verifyElasticsearchFixture(
	ctx context.Context,
	client *goelasticsearch.TypedClient,
	config testenv.ElasticsearchConfig,
) (int, error) {
	if err := testenv.VerifyElasticsearchIndexContract(ctx, client, config); err != nil {
		return 0, err
	}
	query := &types.Query{MatchAll: &types.MatchAllQuery{}}
	ids, err := testenv.QueryElasticsearchIDs(ctx, client, config, query)
	if err != nil {
		return 0, err
	}
	if err := fixture.CompareIDs(ids, fixture.StableIDs()); err != nil {
		return 0, err
	}
	return len(ids), nil
}

type serviceSelection struct {
	sqlBackends   []testenv.Backend
	mongo         bool
	ldap          bool
	elasticsearch bool
}

func selectedServices(value string) (serviceSelection, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "all":
		return serviceSelection{
			sqlBackends: testenv.SQLBackends(), mongo: true, ldap: true,
			elasticsearch: true,
		}, nil
	case "sql":
		return serviceSelection{sqlBackends: testenv.SQLBackends()}, nil
	case "mongo":
		return serviceSelection{mongo: true}, nil
	case "directory", "ldap":
		return serviceSelection{ldap: true}, nil
	case "search", "elasticsearch":
		return serviceSelection{elasticsearch: true}, nil
	default:
		backend, err := testenv.ParseBackend(value)
		if err != nil {
			return serviceSelection{}, err
		}
		return serviceSelection{sqlBackends: []testenv.Backend{backend}}, nil
	}
}
