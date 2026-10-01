# `i18n/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Backend translations use go-i18n catalogs in `locales/` and keys/helpers in `keys.go` and `i18n.go`. Keep translated server messages available in the supported backend locales (`en`, `zh-CN`, `zh-TW`) and preserve key fallback behavior.

Frontend translations are a separate i18next system documented in [../web/default/AGENTS.md](../web/default/AGENTS.md).
