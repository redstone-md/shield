# Multiline Debug Configs Brainstorm

## Current State

- Startup debug logs still print large structs through `%+v`.
- Config structs become very long single lines.
- Pointer addresses in config logs add noise without operator value.

## Target Outcome

- Keep concise log headers from the readable logs change.
- Render debug config structs one field per line.
- Avoid replacing the logging library or touching unrelated runtime logs.

## Options

1. Manually format each config log.
   - Clear for each call site.
   - Repeats formatting logic.

2. Add a small observability formatter for struct fields.
   - Central behavior.
   - Easy to test.
   - Lets call sites stay small.

3. Use JSON indentation for configs.
   - Structured and familiar.
   - Less useful for fields containing interfaces or private pointer values.

## Recommended Direction

Use option 2 for startup debug configs:

- Add `observability.FormatFields`.
- Apply it to OpenAI, Gemini, detector, options, spam bot, and listener config logs.
- Replace the admin handler struct dump with explicit key fields.

## Risks

- Multi-line logs may be split by Docker log presentation, but each field remains readable.
- Some nested fields remain compact if they are not exported structs.
