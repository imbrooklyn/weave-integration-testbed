package testenv

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/imbrooklyn/weave-integration-testbed/internal/fixture"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	defaultMongoHost       = "127.0.0.1"
	defaultMongoPort       = uint16(37017)
	defaultMongoDatabase   = "weave_testbed"
	defaultMongoUser       = "weave"
	defaultMongoPassword   = "weave_demo_only"
	defaultMongoAuthSource = "weave_testbed"
)

// MongoConfig contains the MongoDB endpoint and local test credentials.
// Callers must not log Password.
type MongoConfig struct {
	Host       string
	Port       uint16
	Database   string
	AuthSource string
	User       string
	Password   string
}

// MongoServerInfo is the non-secret server identity returned by buildInfo.
type MongoServerInfo struct {
	Version      string
	VersionArray []int32
}

// LoadMongoConfig reads MongoDB configuration from the documented environment
// variables and applies the same local-only defaults as compose.yaml.
func LoadMongoConfig() (MongoConfig, error) {
	port, err := environmentPort("WEAVE_TESTBED_MONGO_PORT", defaultMongoPort)
	if err != nil {
		return MongoConfig{}, err
	}
	database := environmentValue("WEAVE_TESTBED_MONGO_DATABASE", defaultMongoDatabase)
	config := MongoConfig{
		Host:       environmentValue("WEAVE_TESTBED_MONGO_HOST", defaultMongoHost),
		Port:       port,
		Database:   database,
		AuthSource: environmentValue("WEAVE_TESTBED_MONGO_AUTH_SOURCE", database),
		User:       environmentValue("WEAVE_TESTBED_MONGO_USER", defaultMongoUser),
		Password:   environmentValue("WEAVE_TESTBED_MONGO_PASSWORD", defaultMongoPassword),
	}
	if err := config.validate(); err != nil {
		return MongoConfig{}, err
	}
	return config, nil
}

// Endpoint returns a credential-free service description suitable for logs.
func (config MongoConfig) Endpoint() string {
	return fmt.Sprintf(
		"mongodb://%s/%s",
		net.JoinHostPort(config.Host, strconv.Itoa(int(config.Port))),
		config.Database,
	)
}

// OpenMongo creates an official-driver Client without contacting the server.
// Use WaitForMongo for authenticated readiness.
func OpenMongo(config MongoConfig) (*mongodriver.Client, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	clientOptions := options.Client().
		SetHosts([]string{net.JoinHostPort(config.Host, strconv.Itoa(int(config.Port)))}).
		SetAuth(options.Credential{
			AuthSource:  config.AuthSource,
			Username:    config.User,
			Password:    config.Password,
			PasswordSet: true,
		}).
		SetAppName("weave-integration-testbed").
		SetConnectTimeout(5 * time.Second).
		SetServerSelectionTimeout(5 * time.Second).
		SetMaxPoolSize(64).
		SetMinPoolSize(0)
	client, err := mongodriver.Connect(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("open MongoDB client")
	}
	return client, nil
}

// WaitForMongo retries an authenticated ping until success or context expiry.
// Driver errors are omitted so credentials and connection details cannot leak.
func WaitForMongo(
	ctx context.Context,
	client *mongodriver.Client,
	interval time.Duration,
) error {
	if ctx == nil {
		return fmt.Errorf("wait for MongoDB: nil context")
	}
	if client == nil {
		return fmt.Errorf("wait for MongoDB: nil client")
	}
	if interval <= 0 {
		return fmt.Errorf("wait for MongoDB: interval must be positive")
	}
	for {
		if err := client.Ping(ctx, nil); err == nil {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("MongoDB did not become healthy: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// ReadMongoServerInfo returns buildInfo and rejects servers older than the
// MongoDB60Plus profile baseline.
func ReadMongoServerInfo(
	ctx context.Context,
	client *mongodriver.Client,
) (MongoServerInfo, error) {
	if ctx == nil {
		return MongoServerInfo{}, fmt.Errorf("read MongoDB server info: nil context")
	}
	if client == nil {
		return MongoServerInfo{}, fmt.Errorf("read MongoDB server info: nil client")
	}
	var response struct {
		Version      string  `bson:"version"`
		VersionArray []int32 `bson:"versionArray"`
	}
	err := client.Database("admin").RunCommand(
		ctx,
		bson.D{{Key: "buildInfo", Value: 1}},
	).Decode(&response)
	if err != nil {
		return MongoServerInfo{}, fmt.Errorf("read MongoDB buildInfo")
	}
	if strings.TrimSpace(response.Version) == "" || len(response.VersionArray) < 2 ||
		response.VersionArray[0] < 6 {
		return MongoServerInfo{}, fmt.Errorf("MongoDB server is outside the 6.0+ profile")
	}
	return MongoServerInfo{
		Version:      response.Version,
		VersionArray: append([]int32(nil), response.VersionArray...),
	}, nil
}

// ResetMongo replaces both testbed-owned collections with fresh canonical and
// regex-probe documents while preserving missing fields as absent keys.
func ResetMongo(ctx context.Context, database *mongodriver.Database) error {
	if ctx == nil {
		return fmt.Errorf("reset MongoDB fixture: nil context")
	}
	if database == nil {
		return fmt.Errorf("reset MongoDB fixture: nil database")
	}
	fixtures := []struct {
		collection string
		documents  []bson.D
	}{
		{collection: fixture.MongoCollection, documents: fixture.MongoRecords()},
		{
			collection: fixture.MongoRegexProbeCollection,
			documents:  fixture.MongoRegexProbeRecords(),
		},
	}
	for _, item := range fixtures {
		collection := database.Collection(item.collection)
		if _, err := collection.DeleteMany(ctx, bson.D{}); err != nil {
			return fmt.Errorf("clear MongoDB fixture collection %q", item.collection)
		}
		values := make([]any, len(item.documents))
		for index := range item.documents {
			values[index] = item.documents[index]
		}
		if _, err := collection.InsertMany(ctx, values); err != nil {
			return fmt.Errorf("insert MongoDB fixture collection %q", item.collection)
		}
	}
	return nil
}

// QueryMongoIDs returns string _id values in deterministic order.
func QueryMongoIDs(
	ctx context.Context,
	collection *mongodriver.Collection,
	filter bson.D,
) ([]string, error) {
	if ctx == nil {
		return nil, fmt.Errorf("query MongoDB fixture: nil context")
	}
	if collection == nil {
		return nil, fmt.Errorf("query MongoDB fixture: nil collection")
	}
	if filter == nil {
		return nil, fmt.Errorf("query MongoDB fixture: nil filter")
	}
	cursor, err := collection.Find(
		ctx,
		filter,
		options.Find().
			SetProjection(bson.D{{Key: "_id", Value: 1}}).
			SetSort(bson.D{{Key: "_id", Value: 1}}),
	)
	if err != nil {
		return nil, fmt.Errorf("execute MongoDB fixture query")
	}
	defer cursor.Close(ctx)

	ids := make([]string, 0)
	for cursor.Next(ctx) {
		var result struct {
			ID string `bson:"_id"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("decode MongoDB fixture ID")
		}
		if result.ID == "" {
			return nil, fmt.Errorf("decode MongoDB fixture ID: empty value")
		}
		ids = append(ids, result.ID)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate MongoDB fixture IDs")
	}
	return ids, nil
}

// CloseMongo disconnects a Client with a fresh finite cleanup context.
func CloseMongo(client *mongodriver.Client) error {
	if client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Disconnect(ctx); err != nil {
		return fmt.Errorf("close MongoDB client")
	}
	return nil
}

func (config MongoConfig) validate() error {
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "MongoDB host", value: config.Host},
		{name: "MongoDB database", value: config.Database},
		{name: "MongoDB authentication source", value: config.AuthSource},
		{name: "MongoDB user", value: config.User},
		{name: "MongoDB password", value: config.Password},
	} {
		if strings.TrimSpace(item.value) == "" {
			return fmt.Errorf("%s configuration is empty", item.name)
		}
	}
	if config.Port == 0 {
		return fmt.Errorf("MongoDB port configuration is invalid")
	}
	return nil
}
