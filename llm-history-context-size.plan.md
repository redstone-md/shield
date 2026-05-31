# LLM History Context Size Plan

## Scope

- In scope: detector config, CLI/env option, rule set, settings form/UI, runtime config display, tests.
- Out of scope: reply-to context behavior, slowpath chat reply history, storage migrations.

## Steps

- [x] Add detector tests for default zero history and opt-in recent history.
- [x] Implement detector config gating for LLM history context.
- [x] Add `LLM_HISTORY_CONTEXT_SIZE` option with default `0`.
- [x] Add rule set/common LLM propagation and env precedence.
- [x] Add settings form/UI handling.
- [x] Run focused tests.
- [x] Run related broader tests.

## Exception

- `app/assembly.go` and `app/runtime_assembly.go` already exceed 500 LOC. This change only threads one config value through existing assembly paths; broad runtime assembly splitting is out of scope for this behavior fix.
- `lib/tgspam/detector.go` was reduced to 496 LOC by moving LLM context helpers to `detector_llm_context.go`.

## Results

- Focused detector: `env GOCACHE=/tmp/go-build go test ./lib/tgspam -run 'TestDetector_LLMContext'`
- Focused app/rules/webapi: `env GOCACHE=/tmp/go-build go test ./app ./app/rules ./app/webapi -run 'TestBuildDetectorConfig|TestMakeDetector|TestAssembleRuntimeUsesActiveRuleSet|TestRuleSet|TestApplyExplicitOverrides|TestEnvPinnedKeys|TestRuleSetFromForm'`
- Related full packages: `env GOCACHE=/tmp/go-build go test ./lib/tgspam ./app ./app/rules ./app/webapi`

## History Semantics

- With `LLM_HISTORY_CONTEXT_SIZE=0`, recent chat history is not sent to text LLM checks.
- With `LLM_HISTORY_CONTEXT_SIZE>0`, `lib/tgspam` adds only detector-level ham results to LLM history, excluding `CheckOnly`, empty messages, and detector-level spam.
- Telegram media slow-path runs after `bot.OnMessage`; a message that detector accepted but vision later rejects can already be in detector LLM history. This is pre-existing pipeline ordering and is not changed in this task.

## Verification

- `env GOCACHE=/tmp/go-build go test ./lib/tgspam -run 'TestDetector_LLMContext'`
- `env GOCACHE=/tmp/go-build go test ./app ./app/webapi`
