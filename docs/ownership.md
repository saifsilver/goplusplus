# Ownership

`CODEOWNERS` defines review routing. This document defines responsibility.

| Area | Owner responsibilities |
| --- | --- |
| HTTP kernel and middleware | API compatibility, lifecycle, request safety, performance baselines |
| Authentication | Cryptographic policy, identity installation, compatibility deadlines, security response |
| Database and migrations | Transaction behavior, adapter conformance, migration integrity, failure classification |
| Messaging and queues | Delivery semantics, idempotency, backpressure, replay, shutdown, provider conformance |
| Documentation and examples | Accuracy, runnable examples, readiness labels, migration and release notes |
| Release engineering | CI gates, dependency review, SBOM, provenance, signing, rollback readiness |

At the current project size, `@saifsilver` owns all areas. New maintainers should
be assigned by package in both this document and `.github/CODEOWNERS`. A change
must not be self-approved when another qualified owner is available.
