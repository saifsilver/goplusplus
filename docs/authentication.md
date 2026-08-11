# Production authentication

GoPlusPlus owns cryptographic authentication mechanics and verified request identity. Applications own accounts, endpoints, persistence, account state, normalization, transactions, response policy, secret sources, compatibility decisions, and authorization rules.

## Canonical verified identity

Every successful framework bearer or session path installs the same identity atomically:

- `auth.GetUser(c)` returns cloned, verified `UserClaims`, including ID, subject, email, roles, attributes, and tenant ID.
- `c.UserSubject()` and `c.RequireUserSubject()` return the canonical string subject and support UUIDs and other bounded opaque IDs.
- `c.UserID()` and `c.RequireUserID()` remain compatible numeric accessors. They return no identity for a UUID subject because their public return type is `int64`.
- `auth.RequireRoles` and `auth.RequirePolicy` read the same `UserClaims` object.

Authentication middleware clears the framework identity keys before attempting verification. It never installs identity before signature, registered-claim, compatibility-policy, and identity validation complete. Numeric identities must be positive. String identities must be non-empty, at most 256 bytes, and contain no spaces or control characters. When both ID and subject are present, they must agree.

Applications that set context keys themselves remain source-compatible, but framework verification is the preferred installation path. Do not treat `c.GetString("user_id")` from arbitrary middleware as verified unless that middleware performs equivalent authentication.

## Token manager and strict bearer contract

```go
tokens, err := auth.NewTokenManager(auth.TokenConfig{
    Issuer:      issuer,
    Audience:    audience,
    ActiveKeyID: activeKeyID,
    Keys: map[string][]byte{
        activeKeyID:   activeKey,
        previousKeyID: previousKey, // retain only during bounded rotation
    },
    MaxTTL:    24 * time.Hour,
    ClockSkew: time.Minute,
})
```

Configuration is copied and immutable. Every key must have a non-empty key ID and at least 32 bytes. Issuance uses only `ActiveKeyID`; verification accepts only explicitly listed key IDs. Remove a previous key after all tokens signed by it have expired. The manager never falls back to an unconfigured key or algorithm.

`Authenticate`, `AuthenticateWithManager`, `RequireJWT`, and the bearer branch of `UniversalAuthWithManager` share one parser. It accepts exactly one header with an exact, case-sensitive scheme and one space:

```text
Authorization: Bearer <token>
```

Lowercase or mixed-case schemes, empty tokens, tabs, extra spaces, extra fields, multiple Authorization values, and tokens above 16 KiB are rejected. Exact scheme casing preserves the secure v1.11.1 behavior. Request cancellation is passed through verification. Middleware returns only generic RFC 7807 `401` details: `Missing or invalid bearer token` or `Invalid or expired bearer token`.

Current JWT verification always runs first. HS256, `typ=JWT`, known `kid`, signature, issuer, audience, expiry, not-before, issued-at, JWT ID, maximum lifetime, clock skew, and subject consistency are required. `user_id` is encoded as a positive JSON integer for numeric identities and a JSON string for UUID/opaque identities; it must agree with `sub`. `TokenClaims.UserID` remains the compatible numeric projection and `TokenClaims.UserIDString` carries string identities. A recognized invalid JWT stops the chain.

## Temporary token compatibility

The only built-in historical token adapter is the signed pre-v1.11 GoPlusPlus format:

```text
base64url({"user_id": positive integer, "email": optional, "exp": unix seconds}).base64url(HMAC-SHA256)
```

Enable it only with a strong historical signing key, an explicit deadline, maximum remaining lifetime, and input bound:

```go
LegacyV1: &auth.LegacyTokenConfig{
    SigningKey:   legacyKey,
    AcceptUntil:  sunset,
    MaxTTL:       24 * time.Hour,
    MaxTokenBytes: 1024,
},
```

The adapter is verification-only, strictly decodes JSON/base64url, compares signatures in constant time, and accepts only its historical numeric ID shape. Delete the entire `LegacyV1` configuration after old sessions expire.

Old `v2.jwt...` and `v4.local...` placeholder strings were unsigned. They cannot be authenticated securely and remain rejected. PASETO APIs remain disabled.

For a temporary app-private format, implement only:

```go
type TokenVerifier interface {
    VerifyToken(context.Context, string) (auth.UserClaims, error)
}
```

Register it through `TokenCompatibility` with `AcceptUntil` and `MaxTokenBytes`. Return `auth.ErrTokenFormatNotRecognized` only when the input is unambiguously another format. Any other error means “recognized but invalid” and stops fallback. The framework revalidates returned identity and never exposes adapter errors to clients. Remove the adapter and its secret after the deadline.

## Password policy and timing protection

```go
passwords, err := auth.NewPasswordPolicy(auth.PasswordPolicyConfig{
    Pepper:   passwordPepper,
    Argon2id: auth.DefaultPasswordConfig(),
})
```

The policy copies validated configuration and owns randomized Argon2id PHC hashing, strict bounded parsing, constant-time derived-key comparison, rehash decisions, a randomized dummy hash, and compatibility ordering. Passwords and hashes are limited to 1024 bytes; pepper, memory, iterations, parallelism, salt, and output sizes are bounded before any KDF runs.

`Verify` returns one of:

- `auth.PasswordInvalid`
- `auth.PasswordValid`
- `auth.PasswordValidNeedsRehash`

When account lookup returns no row, call `passwords.VerifyMissing(password)` before returning the same generic `Invalid email or password` response used for a wrong password. The missing path executes one real bounded Argon2id verification. Current stored hashes also execute one Argon2id verification. Tests prove KDF invocation structurally rather than relying on unstable timing thresholds.

Pre-v1.11 GoPlusPlus HMAC password hashes are available only through explicit verification-only compatibility:

```go
LegacyV1: &auth.LegacyPasswordConfig{AcceptUntil: sunset}
```

A successful legacy verification always returns `PasswordValidNeedsRehash`. App-private legacy formats implement `LegacyPasswordVerifier` and are registered with a deadline through `PasswordCompatibility`. Adapters must bound their own internal work, compare secrets in constant time, generate no new legacy hashes, and return `ErrPasswordFormatNotRecognized` only for an unambiguous format mismatch.

## Application-owned hash persistence

The application flow is:

1. Load the account and encoded hash.
2. Call `VerifyMissing` when the account is absent; otherwise call `Verify`.
3. Return the same generic invalid-credentials response for missing accounts and invalid passwords.
4. On `PasswordValidNeedsRehash`, call `Hash`.
5. Persist the replacement in application storage.
6. Prefer compare-and-swap: `UPDATE ... WHERE id = ? AND password_hash = previous_hash`.
7. Continue or fail authentication according to a documented application failure policy.

The framework never updates account records. See [`examples/authentication`](../examples/authentication/application.go) for a concrete `database/sql` implementation.

## Sessions

`SessionMiddleware` and `UniversalAuthWithManager` use the same canonical installer as bearer authentication. Session creation rejects malformed identity before rotating or storing a session. The process-local compatibility manager retains bounded server-side expiry, revocation, cryptographically random rotation, `HttpOnly`, `Secure` by default, explicit `SameSite`, and a bounded store. It is not a Redis client and is not suitable as a shared multi-instance session store.

## Deprecations and limitations

- `VerifyPasswordWithMigration` is deprecated; use `PasswordPolicy.Verify`.
- `HashLegacyPassword` is deprecated and should exist only in migration tests.
- `VerifyLegacyPassword` is deprecated; use bounded `PasswordPolicy` compatibility.
- `GeneratePASETO` and `RequirePASETO` remain disabled.
- `GenerateToken`, `VerifyToken`, `GenerateJWT`, `RequireJWT`, `Authenticate`, and `UniversalAuth` remain compatibility APIs. Prefer configured managers and policies.
- GoPlusPlus does not implement registration, login, account persistence, account-state rules, MFA enrollment, OAuth/OIDC, password reset, lockout, or automatic hash persistence.

Expected authentication failures are not logged by these components. If an application records failures, use a stable category, request ID, and route only—never credentials, tokens, hashes, key IDs, verifier names, or parser errors.
