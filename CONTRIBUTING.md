# Contributing

Thank you for helping improve Weave Integration Testbed.

## Development setup

Install Go 1.27 or newer and Docker Engine with Docker Compose. From the
repository root, run:

```sh
go test ./...
go vet ./...
docker compose --profile sql config --quiet
```

For checks that use the real SQL services:

```sh
docker compose --profile sql up --detach --wait --wait-timeout 180
go run ./cmd/testbedctl check --timeout=2m
go run ./cmd/memory --timeout=2m
go run ./cmd/gormgen --backend=all --timeout=2m
go run ./cmd/gorm --backend=all --timeout=2m
go test -race -tags=integration ./integration
docker compose --profile sql down --volumes --remove-orphans
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
Changes to SQL environment code or fixture data must include:

```sh
test -z "$(gofmt -l .)"
go test ./...
go test -race ./...
go vet ./...
docker compose --profile sql config --quiet
go run ./cmd/testbedctl check --timeout=2m
go run ./cmd/testbedctl check --timeout=2m
go generate .
git diff --exit-code -- internal/gormgenmodel internal/gormgenquery
go run ./cmd/memory --timeout=2m
go run ./cmd/gormgen --backend=all --timeout=2m
go run ./cmd/gorm --backend=all --timeout=2m
go test -race -tags=integration ./integration
```

The repeated check verifies that DDL and seed replay remains deterministic.

## License

By contributing, you agree that your contributions are licensed under the
Apache License 2.0 in [LICENSE](LICENSE).
