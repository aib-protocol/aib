# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| main / latest release | ✅ |
| older releases | ❌ |

## Reporting a Vulnerability

We take the security of AIB Protocol seriously. If you believe you have found a
security vulnerability, please report it privately:

1. Email **security@aib.one** (or use the GitHub
   [Report a vulnerability](https://github.com/aib-protocol/aib/security/advisories/new)
   advisory form).
2. Include a description of the issue, steps to reproduce, and potential impact.
3. Do **not** open a public GitHub issue for security matters.

### What to expect

- Acknowledgement within 72 hours.
- Assessment and a fix timeline communicated to you.
- Credit in the release notes (unless you prefer to remain anonymous).

## Scope

The following are in scope:

- Node, miner, and CLI binaries built from this repository
- Consensus, P2P, and cryptographic code
- Smart contracts in `contracts/`
- Public API endpoints

Out of scope: infrastructure operated by third parties, attacks requiring
physical access, and social engineering.

## Disclosure Policy

We follow coordinated disclosure: vulnerabilities are fixed and disclosed after
a patch is released, with a reasonable window for users to upgrade.
