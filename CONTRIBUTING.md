# Contributing

Thank you for helping improve Weave Integration Testbed.

## Development setup

Install Go 1.27 or newer and Docker Engine with Docker Compose. From the
repository root, run the following after every version in `go.mod` is available
from the configured module source. An unreleased Adapter can be verified with
a repository-external workspace, but a filesystem `replace` must never be
committed here.

```sh
export GOWORK=off
go test ./...
go vet ./...
docker compose --profile all config --quiet
```

For checks that use all current real services:

```sh
export GOWORK=off
docker compose --profile all up --detach --wait --wait-timeout 240
go run ./cmd/testbedctl check --backend=all --timeout=3m
go run ./cmd/memory --timeout=2m
go run ./cmd/gormgen --backend=all --timeout=2m
go run ./cmd/gorm --backend=all --timeout=2m
go run ./cmd/goqu --backend=all --timeout=2m
go run ./cmd/mongo --timeout=2m
go run ./cmd/ldap --timeout=2m
go run ./cmd/elasticsearch --timeout=3m
go test -race -count=1 -tags=integration ./integration
docker compose --profile all down --volumes --remove-orphans
```

Always stop the Compose project when the check finishes, including after a
failure.

## Change guidelines

- Keep the root as one Go module and do not add local filesystem replacements
  to `go.mod`.
- Use only public APIs from Go, database drivers, Weave, and its adapters.
- Do not add adapter compiler implementations to this repository.
- Keep MySQL and PostgreSQL DDL and seed data logically equivalent, with stable
  record IDs.
- Keep MongoDB Extended JSON aligned with documents generated from
  `compilertest.Records()` and preserve missing keys as absent.
- Keep the OpenLDAP custom Schema, typed descriptors, base LDIF, and entries
  generated from `compilertest.Records()` aligned. Do not model an explicit
  LDAP NULL or standard multi-valued field semantics.
- Keep Elasticsearch settings, explicit mapping, NDJSON, immutable Field
  declarations, and the exact 9.5.2 server contract aligned. Do not replace
  mapping assertions with runtime discovery or approximate analyzed,
  multi-valued, nested, NULL, or expensive-query lowering.
- Keep Demo and integration Predicate cases and expected IDs sourced from
  `weave/compilertest`; do not copy them into testbed packages.
- Regenerate the committed GORM Gen model and query with `go generate .` after
  a compatible SQL schema change.
- Make initialization and reset operations deterministic and replayable.
- Keep public code, comments, command output, documentation, and CI text in English.
- Do not commit `.env`, production credentials, private keys, database volumes,
  logs, or generated test output.
- Bind local service ports only as broadly as the demonstration requires.

## Verification

Before submitting a change, run the checks appropriate to the affected files.
Changes to service environment code or fixture data must include:

```sh
export GOWORK=off
test -z "$(gofmt -l .)"
go test ./...
go test -race ./...
go vet ./...
docker compose --profile all config --quiet
go run ./cmd/testbedctl check --backend=all --timeout=2m
go run ./cmd/testbedctl check --backend=all --timeout=2m
go generate .
git diff --exit-code -- internal/gormgenmodel internal/gormgenquery
go run ./cmd/memory --timeout=2m
go run ./cmd/gormgen --backend=all --timeout=2m
go run ./cmd/gorm --backend=all --timeout=2m
go run ./cmd/goqu --backend=all --timeout=2m
go run ./cmd/mongo --timeout=2m
go run ./cmd/ldap --timeout=2m
go run ./cmd/elasticsearch --timeout=3m
go test -race -count=1 -tags=integration ./integration
```

The repeated check verifies that SQL scripts, MongoDB documents, LDAP entries,
and the Elasticsearch index reset deterministically. Real-service tests must
use `-count=1` so a different container version cannot inherit a cached result.

## License

By contributing, you agree that your contributions are licensed under the
Apache License 2.0 in [LICENSE](LICENSE).
