package testenv

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLoadMongoConfigDefaultsAndRedactedEndpoint(t *testing.T) {
	config, err := LoadMongoConfig()
	if err != nil {
		t.Fatalf("LoadMongoConfig() error = %v", err)
	}
	if config.Host != defaultMongoHost || config.Port != defaultMongoPort ||
		config.Database != defaultMongoDatabase || config.AuthSource != defaultMongoAuthSource {
		t.Fatalf("LoadMongoConfig() = %#v", config)
	}
	if strings.Contains(config.Endpoint(), config.Password) ||
		strings.Contains(config.Endpoint(), config.User+"@") {
		t.Fatalf("MongoConfig.Endpoint() exposes credentials: %q", config.Endpoint())
	}
}

func TestLoadMongoConfigRejectsInvalidPortWithoutEchoingValue(t *testing.T) {
	const invalid = "mongo-port-secret"
	t.Setenv("WEAVE_TESTBED_MONGO_PORT", invalid)
	_, err := LoadMongoConfig()
	if err == nil {
		t.Fatal("LoadMongoConfig() error = nil")
	}
	if strings.Contains(err.Error(), invalid) {
		t.Fatalf("LoadMongoConfig() error exposes invalid value: %v", err)
	}
}

func TestLoadMongoConfigDefaultsAuthSourceToSelectedDatabase(t *testing.T) {
	t.Setenv("WEAVE_TESTBED_MONGO_DATABASE", "alternate_test_database")
	t.Setenv("WEAVE_TESTBED_MONGO_AUTH_SOURCE", "")
	config, err := LoadMongoConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.AuthSource != config.Database {
		t.Fatalf("Mongo auth source = %q, want selected database", config.AuthSource)
	}
}

func TestMongoHelpersRejectNilInputs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := WaitForMongo(ctx, nil, time.Millisecond); err == nil {
		t.Fatal("WaitForMongo(nil client) error = nil")
	}
	if err := ResetMongo(ctx, nil); err == nil {
		t.Fatal("ResetMongo(nil database) error = nil")
	}
	if _, err := QueryMongoIDs(ctx, nil, nil); err == nil {
		t.Fatal("QueryMongoIDs(nil collection) error = nil")
	}
}
