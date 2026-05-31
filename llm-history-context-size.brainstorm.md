# LLM History Context Size Brainstorm

## Current state

- Detector always sends up to five recent ham chat messages to text LLM checks.
- `OPENAI_HISTORY_SIZE` and `GEMINI_HISTORY_SIZE` are provider config fields but do not control the shared LLM moderation context.
- The user wants `LLM_HISTORY_CONTEXT_SIZE=0` with default `0`, so recent messages are opt-in.

## Target outcome

- Default LLM moderation requests contain no recent chat history.
- Setting `LLM_HISTORY_CONTEXT_SIZE=N` sends up to `N` recent chat messages.
- Runtime rule sets and UI settings can preserve and display the value.

## Options

1. Change `llmChatContextSize` constant from 5 to 0.
   - Rejected: no env/runtime override, hidden behavior.
2. Add a detector-only config field and CLI/env flag.
   - Partial: enough for env, but DB rule sets would lose live settings.
3. Add a common LLM rule field and pass it through existing config paths.
   - Chosen: consistent with `llm.mode` and `llm.consensus`.

## Risks

- Existing tests expecting five messages need to become opt-in tests.
- Persisted rulesets will unmarshal missing field as zero, matching the requested default.

## Direction

Add `LLMHistoryContextSize` to `tgspam.Config` and `HistoryContextSize` to common LLM rules/options. Only append recent history when the value is positive.
