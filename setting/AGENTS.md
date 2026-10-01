# `setting/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Defines process configuration, runtime option families, provider settings, and synchronized ratio/model/system settings. `setting/config` registers and loads config objects; `operation_setting`, `ratio_setting`, and `billing_setting` hold distinct runtime domains.

Persisted options are loaded and refreshed by `model/option.go`; preserve DB-to-runtime synchronization and validation on both startup and admin updates. Do not add a GORM boolean default when code already enforces the business default. Provider credentials and wallet values are sensitive and must not be logged.

Detailed rules: [ratio_setting/AGENTS.md](ratio_setting/AGENTS.md), [billing_setting/AGENTS.md](billing_setting/AGENTS.md), [operation_setting/AGENTS.md](operation_setting/AGENTS.md).
