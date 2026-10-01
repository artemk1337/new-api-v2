# `service/passkey/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Implements WebAuthn/passkey ceremony logic used by controller handlers. Keep challenge, relying-party, credential ownership, and user-verification rules aligned across registration, login, and verification flows.

Do not move identity or persistence policy into HTTP handlers. Update the adjacent passkey tests for user-visible ceremony contracts when behavior changes.
