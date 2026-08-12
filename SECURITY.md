# Security policy

## Supported versions

Security fixes are provided for the latest tagged minor release. Older minor
releases may receive a fix only when maintainers explicitly announce extended
support. Pre-release, beta, experimental, and development-only components carry
no compatibility or security-support commitment beyond the latest source.

## Reporting a vulnerability

Do not open a public issue. Use GitHub's private vulnerability reporting for
`saifsilver/goplusplus`. If that channel is unavailable, contact the repository
owner through the private contact method published on the owner's GitHub
profile and include "GoPlusPlus security" in the subject.

Include the affected version, reachable component, reproduction, impact,
prerequisites, and any proposed mitigation. Do not include real credentials or
customer data. Maintainers will acknowledge a complete report within five
business days and coordinate validation, remediation, release, and disclosure.

## Security boundaries

Readiness classifications in `docs/readiness.md` are part of this policy.
Process-local helpers are not durable distributed infrastructure. Applications
remain responsible for deployment hardening, secret storage, identity-provider
configuration, data classification, retention, backups, and jurisdictional
requirements.
