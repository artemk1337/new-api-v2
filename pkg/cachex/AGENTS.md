# `pkg/cachex/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Implements cache codecs, namespace handling, and hybrid cache behavior used by application packages. Preserve key namespacing and serialization compatibility across memory/Redis backends; cache failure behavior must not turn a miss into stale authorization or billing state.

Keep this package independent of controllers and provider adapters. Check callers and cache contract tests before changing encodings or expiration semantics.
