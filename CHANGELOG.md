# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- A Go 1.27 root module for the integration testbed.
- A Docker Compose `sql` profile with MySQL 8.0.40 and PostgreSQL 15.12.
- Replayable MySQL and PostgreSQL DDL and stable seed fixtures.
- Environment health, fixture reset, stable ID verification, and cross-backend
  fixture comparison commands.
- English project, contribution, security, and CI foundations.
- Runnable memory, GORM Gen, GORM, and goqu demonstrations with finite timeouts,
  deterministic cleanup, and stable exit behavior.
- A real GORM Gen model and query generated from the shared SQL schema.
- A canonical match-set runner shared by Demos and end-to-end tests, including
  memory-reference comparison across MySQL and PostgreSQL.
- Real prepared-query goqu validation for MySQL and PostgreSQL, including the
  complete compiler contract and memory/GORM Gen/GORM/goqu match-set agreement.
- A `document` Compose profile with fixed MongoDB 6.0.28 defaults, a current
  MongoDB 8.3.8 CI variant, authenticated health checks, and replayable
  Extended JSON initialization that preserves value, explicit null, and missing.
- A runnable mongo Demo and real-server compiler contract covering all 31
  canonical match sets, negative-operator guards, literal PCRE anchors,
  injection boundaries, redacted failures, and concurrent ordered BSON.
- Exact match-set differential checks across MongoDB, memory, GORM Gen, GORM,
  and goqu within their 28-scenario unadjusted storage intersection.
- A `directory` Compose profile pinned to the OpenLDAP 2.6.10 multi-architecture
  manifest digest, with authenticated health checks, custom Schema and base
  LDIF, deterministic entries generated from `compilertest.Records()`, and
  loopback-only access.
- A runnable LDAP Demo and real-server Compiler contract covering the 26
  applicable canonical match sets, exact field applicability, absence guards,
  empty and multi-valued attributes, all four Logic forms, Native/Expr, RFC 4515
  escaping including NUL and injection-like values, structured unsupported
  operations, stable DNs/IDs, and concurrent compilation/search.
- Real LDAP driver-failure checks proving that rejected binds and malformed
  filters do not escape through wrapper errors, plus CI verification of the
  container's actual `slapd` version.

### Changed

- The root module has a tidy dependency graph and no local replacements.
  Released core and Adapter revisions are publicly resolvable; standalone
  `GOWORK=off` LDAP verification additionally requires the declared independent
  LDAP Adapter version to become available.
