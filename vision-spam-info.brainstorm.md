# Vision Spam Info Brainstorm

## Current state

- Media slow-path appends vision results to `bot.Response.CheckResults`.
- Admin ban notifications only show `slowpathReason`, which currently accepts checks named `slowpath`.
- Info callbacks fetch diagnostics from `locator.Spam`; before audit persistence or when locator misses, the UI falls back to `can't get spam info`.
- Manual admin/report flows rebuild diagnostics from text-only messages and can lose image context.

## Target outcome

- Vision spam reasons are visible in ban/warn notifications.
- The info callback can show diagnostics already embedded in the current admin message when locator has no data.
- Media diagnostics keep using existing `CheckResults` plumbing and storage.

## Options

1. Rename all media slow-path checks to `slowpath`.
   - Rejected: loses provider names and changes existing demo output.
2. Teach notification extraction to recognize provider-backed media slow-path details.
   - Chosen: minimal and preserves existing provider diagnostics.
3. Add a separate media-diagnostics store.
   - Rejected: too large for the bug.

## Risks

- Markdown escaping must remain valid in edited admin messages.
- Avoid broad changes to manual ban/report flows in this pass unless tests prove the need.

## Direction

Add regression coverage for provider-named vision results and locator-miss info fallback, then implement focused helpers in `app/events`.
