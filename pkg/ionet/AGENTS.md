# `pkg/ionet/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Contains the OneCloud/Ionet client and typed API responses for container, deployment, and hardware data. Keep remote API details inside this client package and preserve context cancellation, bounded HTTP behavior, and error information for callers.

For endpoint or payload changes, check [../../docs/ionet-client.md](../../docs/ionet-client.md) and update it when the documented client contract changes.
