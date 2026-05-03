# Empty LLM Message Plan

## Scope

In scope:
- Skip text-only LLM checks for blank normalized messages.
- Add regression coverage for image-only fast-path behavior.
- Assert slow-path OpenAI vision request content is multimodal, not null.

Out of scope:
- Telegram transform changes.
- New dependencies.
- Database schema changes.
- Runtime configuration changes.

## Steps

- [x] Add regression test for empty text with short-message OpenAI enabled.
- [x] Add slow-path OpenAI vision request-shape regression test.
- [x] Implement shared blank-message LLM gate.
- [x] Run targeted tests for `lib/tgspam` and `app/slowpath`.
- [x] Run `gofmt` on changed Go files.
- [x] Run broader relevant verification.
- [x] Archive the plan under `docs/plans/completed/` after verification.

## Verification

- `go test ./lib/tgspam -run TestDetector_CheckOpenAI -count=1`
- `go test ./app/slowpath -run TestOpenAIAdapter -count=1`
- Broader relevant package tests after targeted checks pass.
