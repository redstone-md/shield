# 2026-04-22 Action Journal Seam Plan

1. Add `moderation_actions` storage for durable executor command attempts.
2. Introduce explicit executor command names for `delete_message`, `mute_user`, `ban_user`, and `ban_sender_chat`.
3. Wire the action journal into the Telegram action executor and record completed or failed attempts.
4. Expose the store through runtime assembly.
5. Add focused storage, executor, and startup tests.
6. Update roadmap and architecture docs to record the new action-journal seam.

## Validation Skills

- `mcaf-solid-maintainability`: keep command recording at the executor boundary
- `mcaf-testing`: prove storage and executor command journaling work end to end at the seam
