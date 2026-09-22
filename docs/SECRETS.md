# Secret Storage

Provider tokens, proxy passwords, panel credentials, and other sensitive values must never be stored as plaintext in PostgreSQL or committed to Git.

The initial store uses AES-256-GCM authenticated encryption. Ciphertext is bound to account ID, secret ID, and secret kind through associated authenticated data, preventing a ciphertext record from being transparently moved to another account context.

Rules:
- Master encryption keys are supplied outside the repository.
- Plaintext secrets must not be logged or included in error messages.
- Database records contain ciphertext, nonce, and key version only.
- Secret lookup is account-scoped.
- Key versioning is mandatory so rotation can be added without changing the schema.
- Production deployment should source the master key from a protected runtime secret or external KMS/Vault rather than a committed environment file.
