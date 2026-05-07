# Unwarn Command Brainstorm

## Scope

- Add an admin `/unwarn` reply command to remove one manual warning strike.
- Keep the command transport-local in `app/events` and storage-local in `app/storage`.
- Do not change broader policy escalation beyond counting manual warning records where needed.

## Options

- Delete latest `manual_warn` detected-spam row by user ID. Minimal and matches how manual `/warn` records strikes.
- Add a separate warning ledger. Cleaner long-term but larger schema/API change.

## Direction

Use a minimal storage method that deletes the latest manual warning for the subject. Route `/unwarn` like `/warn`: command must reply to a target message, deletes the admin command message, updates warning count, and posts admin confirmation.
