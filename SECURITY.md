# Security Policy

## Supported versions

Security fixes are applied to the current default branch. Version-specific
support information will be published when the testbed begins issuing releases.

## Reporting a vulnerability

Please report suspected vulnerabilities privately through the repository's
GitHub Security Advisories page. Do not open a public issue for a vulnerability
that has not been disclosed responsibly.

Include enough information to reproduce and assess the issue without including
production credentials, personal data, private keys, or other unrelated secrets.

## Test environment credentials

Credentials committed in `compose.yaml` and `.env.example` are public,
non-production demonstration credentials. The services bind only to loopback by
default and use temporary storage. Do not expose this environment to an untrusted
network, and never reuse these credentials elsewhere.

## Native query inputs

Demonstrations that use an adapter's native condition or expression API must use
the upstream parameterization and escaping facilities. The testbed validates its
reviewed examples; it does not make arbitrary native SQL or expressions safe.
