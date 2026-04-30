# 2026-04-13 Text Normalization Seam Plan

1. Add a shared `lib/textnorm` package with configurable normalization stages and a script-fold hook.
2. Rewire detector cleanup and stop-word normalization helpers to use the new package.
3. Keep `app/bot` request shaping behavior stable in this slice.
4. Add focused tests for the normalizer package and rerun detector/bot cleanup coverage.
5. Update roadmap and architecture docs to record the new normalization seam.

## Validation Skills

- `mcaf-solid-maintainability`: pull text cleanup into one reusable boundary
- `mcaf-testing`: prove behavior stays stable for detector cleanup and bot requests
