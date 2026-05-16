# Forward Image Before Delete Brainstorm

## Scope

- In scope: automatic moderation actions in `app/events` that delete image messages after spam detection.
- Out of scope: manual admin report flows, database schema, detector scoring.

## Options

1. Add forwarding in admin report rendering.
   - Pros: close to `[image]` placeholder.
   - Cons: report renderer lacks source chat ID and delete ordering.

2. Add forwarding in action executor before every delete.
   - Pros: centralized delete behavior.
   - Cons: executor does not know whether a message had an image unless all callers pass more context.

3. Add a focused pipeline helper that forwards image messages to admin chat before delete calls.
   - Pros: minimal, has message, source chat, admin chat, and delete ordering.
   - Cons: applies only pipeline-driven moderation deletes.

## Decision

Use option 3. It is the smallest change that satisfies the operational need: before automatic deletion of a spam image, forward the original Telegram message to the admin chat so admins can see the media that caused the ban/restriction/warning.
