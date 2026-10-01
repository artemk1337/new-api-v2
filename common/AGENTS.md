# `common/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Cross-cutting primitives used by application layers: JSON wrappers, HTTP/Gin helpers, Redis and memory cache, environment access, request-body storage, limits, and outbound URL protections. This package is foundational; keep business workflows in `controller`, `service`, or `model` and avoid importing those layers here.

Use `common/json.go` for JSON operations. Reuse the SSRF and outbound request safeguards instead of creating ad hoc network clients. Preserve request-body cleanup and cache key compatibility. Relevant sources include `json.go`, `gin.go`, `ssrf_protection.go`, `request_body.go`, and `redis.go`.
