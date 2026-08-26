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
- Runnable memory, GORM Gen, and GORM demonstrations with finite timeouts,
  deterministic cleanup, and stable exit behavior.
- A real GORM Gen model and query generated from the shared SQL schema.
- A canonical match-set runner shared by Demos and end-to-end tests, including
  memory-reference comparison across MySQL and PostgreSQL.
