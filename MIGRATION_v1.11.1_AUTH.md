# Authentication upgrade from v1.11.1

This upgrade preserves secure v1.11.1 token and password behavior while fixing identity projection and adding explicit compatibility and password-policy modules.

## Move from custom bearer projection

Applications with custom bearer middleware that exists only to populate `"user_id"` can move to `auth.AuthenticateWithManager` after completing these checks:

1. Construct `TokenManager` with the production issuer, audience, active key ID, complete rotation key set, maximum TTL, and clock skew.
2. Confirm numeric routes can call `Context.RequireUserID`. UUID/string-ID routes must call `Context.RequireUserSubject` because the compatible `RequireUserID` return type is `int64`.
3. Configure the signed pre-v1.11 token adapter only if those tokens are still active, and set an explicit acceptance deadline and bounds.
4. Provide a bounded `TokenCompatibility` adapter for an app-private signed format. Do not configure adapters for unsigned tokens.
5. Construct `PasswordPolicy` with the deployment pepper and intended Argon2id parameters.
6. Configure pre-v1.11 HMAC password verification or an app-private password adapter only when stored records require it, always with a deadline.
7. Add integration fixtures for every still-valid deployed token and stored password-hash format.
8. Remove custom context projection, then verify `auth.GetUser`, roles, policies, and the appropriate context identity accessor agree.

## Hash migration

Replace `VerifyPasswordWithMigration` calls with `PasswordPolicy.Verify`. On `PasswordValidNeedsRehash`, create a current hash and update application storage with user ID plus the previous hash as compare-and-swap predicates. The framework does not write storage. On a missing account, call `VerifyMissing` before returning `Invalid email or password`.

## Compatibility removal

After each deadline:

- Remove `LegacyV1` and app-private compatibility entries from configuration.
- Remove historical signing keys and password-verifier secrets from the deployment secret store.
- Delete adapter code once no supported deployment references it.
- Retain an integration test proving expired historical credentials fail closed.

Unsigned pre-v1.11 `v2.jwt...` and `v4.local...` placeholders have no secure migration verifier and remain rejected. Users holding those values must authenticate again through the application's normal login flow.
