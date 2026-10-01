# Frontend source map

Parent: [../AGENTS.md](../AGENTS.md).

`routes/` composes pages, `features/` owns feature UI and API hooks, and `components/`, `hooks/`, `lib/`, `stores/`, and `context/` contain shared frontend code. API transport and global query/auth behavior live in shared modules; feature components should use those modules instead of making independent clients.

See [features/AGENTS.md](features/AGENTS.md) for feature boundaries and the parent document for TypeScript, accessibility, i18n, style, and build rules.
