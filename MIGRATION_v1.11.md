# Migrating to GoPlusPlus v1.11

v1.11 hardens validation, JSON binding, authentication, error responses, and embedded static routing. The changes below intentionally fail closed.

## Validation and request binding

- Validation failures now use `https://goplusplus.dev/errors/validation` and include an ordered `errors` array with JSON field names. Clients that parsed the former free-form `detail` must migrate to `errors`.
- `required` now rejects non-nil empty slices and maps. `omitempty` skips only subsequent rules when empty.
- Validation metadata and traversal are bounded. Unknown rules, malformed parameters, and incompatible rule types return a generic HTTP 500 configuration failure.
- JSON bodies require `application/json` (or a `+json` media type), are limited to 1 MiB, reject unknown fields, and must contain exactly one document.
- Set `app.JSONBinding.AllowUnknownFields`, `AllowNonJSONContentType`, or `MaxBodyBytes` only when compatibility requires it.

## Passwords

- `auth.HashPassword` now creates randomized Argon2id PHC hashes and uses its second argument as an application pepper of 16–1024 bytes.
- `auth.VerifyPassword` accepts only Argon2id hashes.
- Use `auth.VerifyPasswordWithMigration` to authenticate an old HMAC record and detect that the application must replace it. `VerifyLegacyPassword` is migration-only.

## Bearer tokens and sessions

- `GenerateToken` and `GenerateJWT` require exactly one explicit positive TTL and a signing secret of at least 32 bytes. Calls without a TTL return an empty string.
- `Authenticate` verifies signatures and required claims; arbitrary bearer strings no longer create a default administrator identity.
- Prefer `TokenManager` for issuer, audience, key ID, rotation, bounded TTL, and clock-skew policy.
- Placeholder PASETO generation and acceptance are disabled. Use signed JWTs until a complete PASETO implementation is introduced.
- Session identifiers are cryptographically random, rotate during authentication, expire server-side, and set bounded `HttpOnly` cookies. The compatibility `RedisSessionManager` remains process-local; use an application-managed shared store for multi-instance deployments.

For the post-v1.11.1 canonical identity, password policy, and bounded historical-format APIs, continue with [MIGRATION_v1.11.1_AUTH.md](MIGRATION_v1.11.1_AUTH.md).

## Errors and static files

- Unhandled errors and recovered panics are logged with request IDs but return generic Problem Details. Internal `error.Error()` strings are no longer sent to clients.
- `StaticEmbed("/")` is now a fallback after exact and parameterized routes, independent of registration order.
- Non-root mounts strip their URL prefix. Missing `/assets/*` files return 404 instead of the SPA shell.
