# `service/authz/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Owns Casbin authorization policy setup, checks, persistence integration, and periodic policy synchronization across instances. Keep policy semantics here rather than in route handlers; middleware and service use this package for decisions.

Preserve role hierarchy, default policy behavior, and sync lifecycle. `main.go` starts policy synchronization after resources initialize. Changes should be checked against the policy model/configuration and focused authz tests.
