# `setting/ratio_setting/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Owns model, group, cache, image, audio, and pricing-group runtime values and their caches. Preserve option keys and serialized map compatibility because `model/option.go`, relay pricing, and admin APIs share these values.

Pricing-group rename/delete operations must normalize references atomically with option persistence. Invalidate derived/exposed pricing caches whenever source pricing changes. Keep exact zero values and legacy ratio compatibility where existing API contracts depend on them.
