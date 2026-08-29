# Weave Integration Testbed

Weave Integration Testbed provides reproducible services, data, and executable
checks for public Weave Adapter demonstrations and compatibility validation.
The `sql` profile runs MySQL and PostgreSQL, the `document` profile runs
MongoDB, and the `directory` profile runs OpenLDAP. All Adapter paths use the
canonical `weave/compilertest` records, Predicate scenarios, and expected
record-ID sets.

This repository is a local demonstration and validation environment. It is not
a production deployment template.

## Requirements

- Go 1.27 or newer
- Docker Engine with Docker Compose

## Source revision matrix

The root module contains no `replace` directives. Released dependencies are
pinned to exact public VCS versions. The LDAP row declares the independent
Adapter version required by this source tree; that version must be available
from the configured Go module source before the complete matrix is reproducible
with `GOWORK=off`.

| Module | Required version |
| --- | --- |
| `github.com/imbrooklyn/weave` | `v0.1.0-alpha.1.0.20260827162553-afbd6ad7c032` |
| `github.com/imbrooklyn/weave-adapters/memory` | `v0.1.0-alpha.1.0.20260828122433-e11008af9c41` |
| `github.com/imbrooklyn/weave-adapters/gormgen` | `v0.1.0-alpha.1.0.20260828122433-e11008af9c41` |
| `github.com/imbrooklyn/weave-adapters/gorm` | `v0.1.0-alpha.1.0.20260828122433-e11008af9c41` |
| `github.com/imbrooklyn/weave-adapters/goqu` | `v0.0.0-20260828122433-e11008af9c41` |
| `github.com/imbrooklyn/weave-adapters/mongo` | `v0.0.0-20260828122433-e11008af9c41` |
| `github.com/imbrooklyn/weave-adapters/ldap` | `v0.1.0-alpha.1` |

CI disables workspace resolution. Once every required version above is
available, verify the same dependency boundary locally before starting
services:

```sh
export GOWORK=off
go mod verify
go mod tidy -diff
```

At the current unreleased source state, `ldap/v0.1.0-alpha.1` is not yet a
publicly resolvable module revision. Commands that load the LDAP packages will
therefore fail under `GOWORK=off` until that independent module version exists;
do not add a committed filesystem replacement to bypass this boundary.

## Quick start

Start all current services and wait for authenticated health checks:

```sh
docker compose --profile all up --detach --wait --wait-timeout 180
```

Reset and verify all four service fixtures:

```sh
go run ./cmd/testbedctl check --backend=all --timeout=2m
```

Run the memory reference, SQL Demos, MongoDB Demo, LDAP Demo, and complete
real-service matrix. `-count=1` prevents a previous service result from being
reused through the Go test cache.

```sh
go run ./cmd/memory --timeout=2m
go run ./cmd/gormgen --backend=all --timeout=2m
go run ./cmd/gorm --backend=all --timeout=2m
go run ./cmd/goqu --backend=all --timeout=2m
go run ./cmd/mongo --timeout=2m
go run ./cmd/ldap --timeout=2m
go test -race -count=1 -tags=integration ./integration
```

Stop the isolated environment:

```sh
docker compose --profile all down --volumes --remove-orphans
```

Every Demo has a finite timeout, closes its client or database pool before
exiting, and returns a nonzero status when setup, compilation, execution, or an
ID-set assertion fails. No external secret or interactive step is required.
See [cmd/README.md](cmd/README.md) for per-Adapter details.

## Runnable Adapter matrix

| Demo | Backend | Public execution path | Canonical result |
| --- | --- | --- | --- |
| `cmd/memory` | In-process | memory typed fields and `Condition.Match` | 31 passed, 0 skipped |
| `cmd/gormgen` | MySQL, PostgreSQL | generated DAO `Where` | 30 passed, 1 skipped |
| `cmd/gorm` | MySQL, PostgreSQL | typed GORM fields and `DB.Where` | 30 passed, 1 skipped |
| `cmd/goqu` | MySQL, PostgreSQL | typed goqu fields and prepared `database/sql` queries | 30 passed, 1 skipped |
| `cmd/mongo` | MongoDB 6.0+ | typed Mongo paths and `Collection.Find` with ordered BSON | 31 passed, 0 skipped |
| `cmd/ldap` | OpenLDAP 2.6.10 | typed Schema descriptors and `SearchRequest.Filter` | 26 passed, 0 skipped |

The commands print each scenario name and its final sorted record-ID set. They
do not compare backend query text or serialize credentials and stored text into
the report. SQL Demos accept `--backend=mysql`, `--backend=postgres`, or the
default `--backend=all`. `--timeout` applies independently to each selected
service.

## Environment commands

`testbedctl` accepts `--backend=all`, `--backend=sql`, `--backend=mysql`,
`--backend=postgres`, `--backend=mongo`, `--backend=directory`, or
`--backend=ldap`. The default `all` selects every current service. Each
operation has a finite per-service timeout.

```sh
go run ./cmd/testbedctl health --backend=all --timeout=2m
go run ./cmd/testbedctl reset --backend=all --timeout=2m
go run ./cmd/testbedctl verify --backend=all --timeout=2m
go run ./cmd/testbedctl check --backend=all --timeout=2m
```

- `health` performs authenticated service checks and reports real MongoDB and
  pinned OpenLDAP identities.
- `reset` replays SQL scripts or inserts fresh ordered BSON and LDAP entries.
- `verify` checks the stable record-ID set.
- `check` resets and verifies each selected service. When both SQL backends are
  selected, it also compares every shared SQL fixture column without logging
  stored values.

## Service profiles

| Profile | Service | Default image | Host endpoint | Database |
| --- | --- | --- | --- | --- |
| `sql` | MySQL | `mysql:8.0.40` | `127.0.0.1:33306` | `weave_testbed` |
| `sql` | PostgreSQL | `postgres:15.12-alpine` | `127.0.0.1:35432` | `weave_testbed` |
| `document` | MongoDB | `mongo:6.0.28` | `127.0.0.1:37017` | `weave_testbed` |
| `directory` | OpenLDAP | `bitnamilegacy/openldap:2.6.10-debian-12-r4@sha256:966fd39ed25813890e9bd57dac56def163bbcfe64967e0bae59ab018d505bd93` | `127.0.0.1:3389` | `dc=weave,dc=test` |

`all` enables all three current profiles. Only service ports are published and
each is bound to the host loopback interface. Storage remains container-local,
so removing the Compose project removes test data.

The MongoDB profile can run another fixed compatible image without editing the
Compose file. The real-service matrix validates both the 6.0 baseline and the
current stable 8.3 line:

```sh
WEAVE_TESTBED_MONGO_IMAGE=mongo:8.3.8 \
  docker compose --profile document up --detach --wait --wait-timeout 180
```

The committed defaults use the public local-only account `weave` with password
`weave_demo_only`. These credentials are intentionally non-secret and must
never be reused for production or a network-accessible database. Copy
`.env.example` to `.env` to change local endpoints or credentials; `.env` is
ignored by Git.

The directory profile uses the same public local-only password for
`cn=admin,dc=weave,dc=test`, disables anonymous binding, and exposes only the
loopback port. The pinned `bitnamilegacy` image is an immutable validation
fixture, receives no security updates, and is not a deployment recommendation.

## Stable fixture

Every service exposes the six canonical IDs `r01` through `r06`. SQL DDL and
seed files include the wider testbed row shape. MongoDB `records.json` stores
the canonical scalar fields as Extended JSON and is checked structurally
against documents generated from `compilertest.Records()`.

The directory profile loads the committed custom Schema and base LDIF, then
materializes canonical entries directly from `compilertest.Records()` on every
reset. Its optional nullable attributes collapse explicit null and missing to
LDAP absence. LDAP-only probes cover a multi-valued attribute, a present empty
IA5 value, an absent IA5 attribute, arbitrary octets containing NUL, literal
filter delimiters, Unicode, and an injection-like string.

The Mongo Demo and automated tests do not own a second scenario or expected-ID
list: both execute `compilertest.Scenarios`, and the runtime Mongo documents are
materialized from `compilertest.Records()`. The committed Extended JSON is an
initialization artifact whose equality with that shared fixture is tested.

The MongoDB fixture preserves all three nullable states:

- r01, r02, r05, and r06 contain non-null values;
- r03 contains explicit BSON null;
- r04 omits both nullable keys completely.

The separate `regex_probe_records` collection contains only real-server seam
probes for literal metacharacters, backslashes, Unicode, embedded and trailing
newlines, absolute subject anchors, and an injection-like string value. It does
not replace or duplicate canonical Predicate scenarios.

SQL materializes explicit null and an unavailable field as SQL `NULL`. Its
canonical `explicit null only` and nullable-membership results therefore use
the documented missing-collapsed sets, and `missing state` is skipped. MongoDB
and memory preserve the states and execute all 31 scenarios. The remaining 28
unadjusted scenarios are compared exactly across MongoDB, GORM Gen, GORM, and
goqu by final ID set.

The committed fixture sources are:

```text
testdata/mysql/001_schema.sql
testdata/mysql/002_seed.sql
testdata/postgres/001_schema.sql
testdata/postgres/002_seed.sql
testdata/mongo/records.json
testdata/mongo/regex_records.json
testdata/mongo/init.js
testdata/directory/schema/00-weave.ldif
testdata/directory/ldif/00-base.ldif
```

## MongoDB semantic checks

The MongoDB harness uses the official Go Driver v2.8.2, verifies `buildInfo`
reports MongoDB 6.0 or newer, and passes each compiled `bson.D` directly to
`Collection.Find`. It checks every standard Operator, constants, empty groups,
all four Logic forms, three-level nesting, nullable membership, Native, Expr,
validation failures, stable first error, and concurrent execution.

Dedicated real-service regressions compare bare MongoDB `$ne` and `$nin`
filters with the guarded Weave forms. Bare negative operators include explicit
null and missing records according to MongoDB rules; the compiled filters must
match the memory-reference set instead. The tests also prove that `IsNull`
selects only r03, `NotNull` selects only present non-null records, and logical
negation preserves the binary match-set complement.

Literal Contains, HasPrefix, and HasSuffix are executed with regex
metacharacters, a backslash, Unicode, and newlines. The emitted `$regex` value
must be one quoted string with no caller-controlled options. A trailing-newline
probe distinguishes raw `$`, which can match before a final newline, from the
Adapter's absolute `\z` suffix anchor.

Typed path injection attempts are rejected before execution. An operator-like
query string is retained as one BSON value and can match only an identical
stored string. Failed compilation returns a nil document and redacted
structured error. Repeated and concurrent compilation must produce identical
ordered BSON bytes and final IDs.

## LDAP semantic checks

The LDAP harness uses go-ldap v3.4.14, verifies LDAPv3 and the custom attribute
OIDs through the live root DSE/subschema, resets entries through authenticated
LDAP operations, and passes each canonical `Filter.String()` directly to
`NewSearchRequest`. The Compose manifest and container `slapd` binary are fixed
at OpenLDAP 2.6.10.

The live Compiler contract executes the 26 canonical cases applicable to the
Adapter's exact global and field capabilities. Dedicated checks cover equality,
integer ordering, presence, substring matching, empty IA5 equality, a
multi-valued Expr, absence-safe NEQ/NotIn, NOT match-set complements, all four
Logic forms, three-level nesting, Schema-bound Native, validated Expr, stable
DN/ID sets, and concurrent compilation/search.

Special-value searches prove that `*`, `(`, `)`, backslash, NUL, Unicode, and
an injection-like string remain escaped assertion values. IsNull, LT, GT, and
standard operators on the multi-valued descriptor return zero filters with
structured errors and are never sent to the server as approximations. The
typed descriptors must agree with the deployed attribute OIDs, `SINGLE-VALUE`
flags, syntaxes, and equality/ordering/substring matching rules.

Real driver-failure checks use a rejected bind and a malformed search filter
to verify that wrapper errors omit credentials, bind identities, and filter
text. Successful filters remain caller-owned query data and are not printed by
the harness.

## Generated GORM Gen fixture

`internal/gormgenmodel` and `internal/gormgenquery` are generated by
`gorm.io/gen` from the live `semantic_records` SQL table. With the SQL profile
healthy, regenerate both packages with:

```sh
go generate .
```

Generated files are committed. A compatible SQL schema change must regenerate
both packages and keep the DDL, seed, Demo, and integration matrix aligned.

## End-to-end tests

Run only the document contract against a healthy MongoDB service:

```sh
go test -race -count=1 -tags=integration -run '^TestMongo' ./integration
```

Run only the directory contract against a healthy OpenLDAP service:

```sh
go test -race -count=1 -tags=integration -run '^TestLDAP' ./integration
```

Run only the full cross-model ID-set comparison with all current services:

```sh
go test -race -count=1 -tags=integration \
  -run '^TestCrossModelMatchSetMatrix$' ./integration
```

The complete package also retains the SQL matrix and goqu compiler contract.
No test treats BSON snapshots, generated SQL, or query text as a substitute for
real backend match sets.

## Project boundaries

- Environment and query wiring use public Go, driver, Weave, and Adapter APIs.
- The testbed executes compiled conditions and compares final record-ID sets.
- The testbed does not implement Adapter compilers.
- Service credentials, schemas, and data are for demonstration and validation only.

## License

Weave Integration Testbed is licensed under the Apache License 2.0. See
[LICENSE](LICENSE).

Apache-2.0 applies to this repository's own content. Referenced container
images and downloaded Go modules remain independent third-party software under
their respective licenses and notices; this project does not relicense them.
Review the terms shipped with the exact third-party version before
redistributing those artifacts.
