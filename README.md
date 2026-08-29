# Weave Integration Testbed

Weave Integration Testbed provides reproducible services, data, and executable
checks for public Weave Adapter demonstrations and compatibility validation.
The `sql` profile runs MySQL and PostgreSQL, the `document` profile runs
MongoDB, the `directory` profile runs OpenLDAP, and the `search` profile runs
Elasticsearch. All Adapter paths use the canonical `weave/compilertest`
records, Predicate scenarios, and expected record-ID sets.

This repository is a local demonstration and validation environment. It is not
a production deployment template.

## Requirements

- Go 1.27 or newer
- Docker Engine with Docker Compose

## Source revision matrix

The root module contains no `replace` directives. Dependencies are pinned to
exact public VCS versions. Every version in the matrix is publicly resolvable
and supports reproducing the complete module graph with `GOWORK=off`.

| Module | Required version |
| --- | --- |
| `github.com/imbrooklyn/weave` | `v0.1.0-alpha.1.0.20260829054240-e89df665411b` |
| `github.com/imbrooklyn/weave-adapters/memory` | `v0.1.0-alpha.1.0.20260828122433-e11008af9c41` |
| `github.com/imbrooklyn/weave-adapters/gormgen` | `v0.1.0-alpha.1.0.20260828122433-e11008af9c41` |
| `github.com/imbrooklyn/weave-adapters/gorm` | `v0.1.0-alpha.1.0.20260828122433-e11008af9c41` |
| `github.com/imbrooklyn/weave-adapters/goqu` | `v0.0.0-20260828122433-e11008af9c41` |
| `github.com/imbrooklyn/weave-adapters/mongo` | `v0.0.0-20260828122433-e11008af9c41` |
| `github.com/imbrooklyn/weave-adapters/ldap` | `v0.0.0-20260829101820-6d007b0c78ef` |
| `github.com/imbrooklyn/weave-adapters/elasticsearch` | `v0.0.0-20260829101820-6d007b0c78ef` |
| `github.com/elastic/go-elasticsearch/v9` | `v9.5.1` |

CI disables workspace resolution. Verify the same dependency boundary locally
before starting services:

```sh
export GOWORK=off
go mod verify
go mod tidy -diff
```

The LDAP and Elasticsearch modules are independent module paths pinned to the
same public source revision. Do not add a committed filesystem replacement to
bypass this boundary.

## Quick start

Start all current services and wait for health checks:

```sh
docker compose --profile all up --detach --wait --wait-timeout 240
```

Reset and verify all five service fixtures:

```sh
go run ./cmd/testbedctl check --backend=all --timeout=2m
```

Run the memory reference, SQL Demos, MongoDB Demo, LDAP Demo, Elasticsearch
Demo, and complete real-service matrix. `-count=1` prevents a previous service
result from being reused through the Go test cache.

```sh
go run ./cmd/memory --timeout=2m
go run ./cmd/gormgen --backend=all --timeout=2m
go run ./cmd/gorm --backend=all --timeout=2m
go run ./cmd/goqu --backend=all --timeout=2m
go run ./cmd/mongo --timeout=2m
go run ./cmd/ldap --timeout=2m
go run ./cmd/elasticsearch --timeout=3m
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

| Demo | Backend | Public execution path | Result |
| --- | --- | --- | --- |
| `cmd/memory` | In-process | memory typed fields and `Condition.Match` | 31 passed, 0 skipped |
| `cmd/gormgen` | MySQL, PostgreSQL | generated DAO `Where` | 30 passed, 1 skipped |
| `cmd/gorm` | MySQL, PostgreSQL | typed GORM fields and `DB.Where` | 30 passed, 1 skipped |
| `cmd/goqu` | MySQL, PostgreSQL | typed goqu fields and prepared `database/sql` queries | 30 passed, 1 skipped |
| `cmd/mongo` | MongoDB 6.0+ | typed Mongo paths and `Collection.Find` with ordered BSON | 31 passed, 0 skipped |
| `cmd/ldap` | OpenLDAP 2.6.10 | typed Schema descriptors and `SearchRequest.Filter` | 26 passed, 0 skipped |
| `cmd/elasticsearch` | Elasticsearch 9.5.2 / Lucene 10.5.1 | official typed Query passed to `TypedClient.Search` | 31 canonical plus 16 search seams, 0 skipped |

The commands print each scenario name and its final sorted record-ID set. They
do not compare backend query text or serialize credentials and stored text into
the report. SQL Demos accept `--backend=mysql`, `--backend=postgres`, or the
default `--backend=all`. `--timeout` applies independently to each selected
service.

## Environment commands

`testbedctl` accepts `--backend=all`, `--backend=sql`, `--backend=mysql`,
`--backend=postgres`, `--backend=mongo`, `--backend=directory`, or
`--backend=ldap`, `--backend=search`, or `--backend=elasticsearch`. The default
`all` selects every current service. Each operation has a finite per-service
timeout.

```sh
go run ./cmd/testbedctl health --backend=all --timeout=2m
go run ./cmd/testbedctl reset --backend=all --timeout=2m
go run ./cmd/testbedctl verify --backend=all --timeout=2m
go run ./cmd/testbedctl check --backend=all --timeout=2m
```

- `health` performs service checks and reports real MongoDB, pinned OpenLDAP,
  and exact Elasticsearch/Lucene identities.
- `reset` replays SQL scripts, inserts fresh ordered BSON and LDAP entries, or
  recreates the explicit Elasticsearch mapping and bulk fixture.
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
| `search` | Elasticsearch | `docker.elastic.co/elasticsearch/elasticsearch:9.5.2@sha256:9c1e1afc2bda921b35025e21c72ec6e392266995aa35ad6a47887363592718be` | `127.0.0.1:39200` | `weave-semantic-records` |

`all` enables all four current profiles. Only service ports are published and
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

The search profile disables Elasticsearch security only inside this isolated,
loopback-published fixture. It pins the multi-platform server image manifest,
verifies Elasticsearch 9.5.2 and Lucene 10.5.1, and uses a tmpfs data
directory. It is not suitable for a shared host or production deployment.

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

The search profile recreates one strict index from committed settings, an
explicit mapping, and NDJSON. Fixture validation compares its canonical fields
with `compilertest.Records()`; Demo and integration code share the same mapping,
data loader, and Elasticsearch seam-case runner instead of copying the
canonical scenario contract. Extra fields cover keyword and analyzed text,
long/double/date/boolean scalars, arrays, source null, missing keys, an empty
string, an empty array, same-field `null_value`, and companion NULL/Value
markers.

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
memory, and Elasticsearch preserve the states and execute all 31 scenarios.
The remaining 28 unadjusted scenarios are compared exactly across MongoDB,
GORM Gen, GORM, and goqu by final ID set.

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
testdata/elasticsearch/settings.json
testdata/elasticsearch/mapping.json
testdata/elasticsearch/records.ndjson
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

## Elasticsearch semantic checks

The search harness uses go-elasticsearch v9.5.1 and refuses any server identity
other than Elasticsearch 9.5.2 with Lucene 10.5.1. It creates a strict explicit
mapping using the `strict_date_optional_time_nanos` date format, a lowercase
keyword normalizer, a wildcard field, same-field `null_value` sentinels, and
companion marker fields. The Adapter Compiler receives only immutable
declarations constructed by the harness; it never discovers cluster mappings
or owns the client, transport, index, context, credentials, or request builder.

The live contract executes all 31 canonical match sets plus 16 shared search
seams. Coverage includes EQ/NEQ, order, In/NotIn, numeric Between, all four
Logic forms, constants, exists guards, source null versus indexed existence,
missing, empty string, empty array, both NULL-marker strategies, root Native,
upstream Expr, analyzed match queries, depth-eight nesting, stable IDs, and
redacted stable-first validation errors. Date intervals use GTE and LTE because
the locked core API exposes Between only for numeric values.

Literal Contains and HasSuffix escape caller `*`, `?`, and backslash before
adding Adapter-owned wildcard operators; Unicode remains literal. HasPrefix
uses a typed prefix query. With `search.allow_expensive_queries=false`, keyword
pattern capabilities are removed and a raw expensive Expr is confirmed to fail
at the real server. After the setting is explicitly enabled and verified, the
expensive Profile executes keyword wildcard and prefix cases. The wildcard
mapping retains its standard literal-pattern capabilities in the strict
Profile.

Analyzed, multi-valued, nested, incomplete-null, and disallowed-expensive
declarations lose standard capabilities instead of receiving approximate
lowering. Full-text, nested, geo, script, and query-string behavior remains an
explicit upstream Expr or future Elasticsearch-specific helper concern.

Failed Elasticsearch requests are wrapped without endpoint text, query JSON,
response bodies, stored values, or credentials. Successful typed queries
remain caller-owned request data and are not printed by the harness.

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

Run only the search contract against a healthy Elasticsearch service:

```sh
go test -race -count=1 -tags=integration \
  -run '^TestElasticsearch' ./integration
```

Run only the full cross-model ID-set comparison with all current services:

```sh
go test -race -count=1 -tags=integration \
  -run '^TestCrossModelMatchSetMatrix$' ./integration
```

The complete package also retains the SQL matrix and goqu compiler contract.
No test treats BSON snapshots, generated SQL, typed-query JSON, or query text as
a substitute for real backend match sets.

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

The locked go-elasticsearch client is Apache-2.0 software. That does not apply
the same license to the Elasticsearch server distribution: Elastic documents
the default distribution under the Elastic License 2.0, and the pinned 9.5.2
image ships `/usr/share/elasticsearch/LICENSE.txt` and `NOTICE.txt`. Compose
downloads and runs that image for local validation; this repository does not
copy or redistribute its layers. Review the [Elastic licensing FAQ](https://www.elastic.co/pricing/faq/licensing)
and the files shipped in the image before any redistribution or non-test use.
