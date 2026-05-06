# Slowpath Reason Notifications Brainstorm

## Scope

- Add slowpath LLM reason to public auto-warning messages.
- Add slowpath LLM reason to admin-group notifications for automatic warnings and automatic punishments.
- Keep existing moderation contracts and Telegram action flow unchanged.

## Options

- Add a new field to policy outcome or bot response. Rejected: wider contract change is unnecessary because slowpath already stores the reason in `CheckResults`.
- Parse `CheckResults` at notification formatting sites. Chosen: smallest change, no storage/schema changes.

## Direction

- Add a helper that returns the `Details` from the spam `CheckResults` entry named `slowpath`.
- Append `Причина: <reason>` only when that helper returns a non-empty value.
- Pass the extracted reason from the pipeline into admin notification methods for automatic actions.
