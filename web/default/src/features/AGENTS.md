# Frontend features

Parent: [../AGENTS.md](../AGENTS.md).

Each `features/<name>/` area owns its page-level components and feature-specific hooks, API types, and utilities. Keep reusable primitives in `src/components` or `src/lib`; do not couple one feature to another feature's private implementation.

Split large TSX modules at stable responsibilities such as independent dialogs, forms, editors, or domain hooks. Preserve props and callback contracts when extracting components. User-facing text must continue through i18next and all six locales; follow the parent frontend rules.

Complex feature guidance: [system-settings/AGENTS.md](system-settings/AGENTS.md).
