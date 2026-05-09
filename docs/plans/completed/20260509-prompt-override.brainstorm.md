# Slowpath Prompt Override Brainstorm

## Scope

- In scope: allow operators to supply a full custom slowpath system prompt from `prompt-override.md`; document required prompt contract in `README.md`.
- Out of scope: database prompt registry, web UI management, hot reload, provider-specific prompt files.

## Current State

- `app/slowpath` has a `PromptRegistry`, but `makeSlowPathEngine` does not set it.
- `--openai.prompt` and `--gemini.prompt` exist and are used by the legacy detector LLM checks.
- Slowpath text checks fall back to `defaultSystemPrompt` when no registry prompt is supplied.

## Options

1. Add a new CLI option for prompt file path.
2. Use a fixed file in `FILES_DYNAMIC` named `prompt-override.md`.
3. Replace the built-in default prompt only.

## Decision

- Use option 2 for minimal operator friction and Docker compatibility with the existing `/srv/data` mount.
- Precedence: provider CLI/env prompt, then `prompt-override.md`, then built-in default.
- Apply the same file to OpenAI and Gemini slowpath providers when configured.
