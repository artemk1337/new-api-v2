# `relay/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Coordinates the upstream request lifecycle: relay handlers resolve channel adapters, build provider requests, stream or collect responses, and report usage for settlement. Provider implementations live in [channel/AGENTS.md](channel/AGENTS.md); shared primitives are in `common`, `common_handler`, `helper`, and `constant`. See [common/AGENTS.md](common/AGENTS.md) for override and context contracts.

Preserve request context and billing snapshots from middleware/controller through provider execution and settlement. Streaming paths must preserve cancellation, flush/event semantics, and usage/error reporting. Provider capability changes must update channel registration and stream support declarations where applicable.

After a client disconnect, the shared SSE scanner may collect final usage for at most two seconds without writing downstream. Keep `client_gone` diagnostics, close the upstream body before waiting for workers, and finish queued usage handling before returning to settlement.
The scanner owns this grace period. Cancel the upstream HTTP context when its response body closes, rather than directly on client disconnect, so final billable usage can still arrive.

Optional client scalar values must preserve absence versus explicit zero/false when converted to provider DTOs. Read `pkg/billingexpr/expr.md` before changing expression-based billing data flow.
