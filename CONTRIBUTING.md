# Contributing to GoPlusPlus

GoPlusPlus accepts focused changes that improve correctness, security,
maintainability, documentation, or measured performance. Open an issue before
large API or architecture changes so maintainers can agree on scope.

## Development workflow

1. Create a branch from the supported development branch.
2. Keep each pull request to one independently reviewable change.
3. Add or update tests for behavior changes and regressions.
4. Update public API documentation, examples, migration notes, and ADRs in the
   same pull request when their contracts change.
5. Run `make verify` before requesting review.

New exported APIs require a usage example, doc comments describing defaults and
failure behavior, and a compatibility assessment. New infrastructure adapters
must implement the relevant contract tests and declare their readiness level in
`docs/readiness.md`.

## Review expectations

Pull requests must explain the user-visible behavior, security implications,
failure modes, compatibility impact, and verification performed. Reviewers may
request that unrelated refactoring or large changes be split into separate pull
requests.

Do not include credentials, production data, customer identifiers, or private
incident details in issues, tests, fixtures, logs, or commits. Report security
issues using `SECURITY.md` instead of a public issue.

By contributing, you agree that your contributions are licensed under the MIT
License in `LICENSE`.
