# `scripts/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Repository scripts support local development, migrations/demo data, or release operations. Make destructive effects explicit, scope data changes to named resources, and keep scripts safe to rerun where possible.

Production scripts must use already-built images and follow backup/volume safeguards in the root [AGENTS.md](../AGENTS.md). Document required environment variables and expected working directory in the script header or adjacent docs.
