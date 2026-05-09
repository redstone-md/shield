# Slowpath Prompt Override Plan

## Done Criteria

- [x] `makeSlowPathEngine` loads `prompt-override.md` from the dynamic data directory.
- [x] Slowpath text checks use explicit provider prompt first, file override second, built-in default last.
- [x] Tests cover file loading, missing file, and explicit prompt precedence.
- [x] README documents where to put the file and what the prompt must preserve.
- [x] Relevant tests and formatting pass.

## Steps

- [x] Add prompt override filename constant and loader helper.
- [x] Wire a prompt registry into slowpath assembly when prompts are configured.
- [x] Add assembly-level tests for prompt resolution behavior.
- [x] Update README with custom prompt contract.
- [x] Run `gofmt`, targeted tests, and final build/test commands.
