# `middleware/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Middleware handles request identity, token and user authorization, limits, channel distribution, request metadata, and common response policy. It runs before controller handlers and is attached in `router/`.

Authentication, model/group eligibility, rate limits, payment creation limits, and distributor ordering protect access and accounting. Keep those checks in the request pipeline rather than duplicating them in individual handlers. Preserve context keys consumed by `controller`, `relay`, and `service`.

Key flows are in `auth.go`, `token.go`, `rate-limit.go`, and `distributor.go`; test externally visible access and routing contracts.
