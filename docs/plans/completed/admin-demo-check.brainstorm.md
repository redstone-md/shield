# Admin Demo Check Brainstorm

## Scope

In scope: allow a superuser to post a normal message in the admin chat and receive a spam-detection diagnostic without deleting, banning, warning, training, or changing approved users.

Out of scope: new Telegram commands, web UI changes, database schema changes, public API changes.

## Options

1. Reuse the full moderation queue and add a dry-run execution mode per event.
2. Add a narrow admin-chat diagnostic path that calls the detector with `checkOnly=true` and formats the same check results.

## Decision

Use option 2. It is smaller, avoids idempotency/audit side effects, and matches the existing admin-forward diagnostic pattern while preserving the forward-message moderation flow.
