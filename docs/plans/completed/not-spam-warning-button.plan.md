# Not Spam Warning Button Plan

## Done Criteria

- Warning admin notifications include a `Не спам` inline button.
- Clicking `Не спам` replaces the markup with confirmation controls.
- Confirming records the warning message text as ham and removes markup.
- Confirming clears all warning strikes for the warned user.
- Existing ban/unban callbacks keep current behavior.
- Admin-chat reply `/unwarn` removes a warning instead of being analyzed as an admin demo/check message.
- Relevant tests pass.

## Steps

- [x] Add failing tests for warning markup and confirmation callback ham update.
- [x] Add warning-specific callback prefixes and dispatch.
- [x] Add helper to extract the warned message text from warning admin notifications.
- [x] Wire `ReportWarn` markup to the new callback.
- [x] Make confirmed warning ham remove all warning/spam strikes for the user.
- [x] Handle admin-chat reply `/unwarn` before generic admin message analysis.
- [x] Fix warning removal fallback so automatic warning strikes are removed when no `manual_warn` entry exists.
- [x] Run targeted `app/events` tests.
- [x] Run formatting and relevant verification.
- [x] Archive completed plan under `docs/plans/completed/`.

## Verification

- `go test ./app/events ./app/storage`
