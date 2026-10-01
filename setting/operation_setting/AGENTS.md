# `setting/operation_setting/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Owns runtime operations and feature/payment lifecycle configuration, including the configured payment-method catalog and status-code policies. Keep parsing and canonicalization compatible with persisted option values and admin API behavior.

Payment methods determine which checkout flows are visible and active; preserve provider-specific validation and do not infer activation from credentials alone. Sensitive values are redacted through the appropriate settings helpers.
