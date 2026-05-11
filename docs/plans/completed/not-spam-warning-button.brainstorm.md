# Not Spam Warning Button Brainstorm

## Problem

Admin warning messages currently report false-positive warning strikes but do not expose a direct ham-training action. Admins need a `Не спам` button on warning notifications, with a confirmation step, that records the warned message as ham and clears warning strikes.

## Scope

- In scope: admin-chat `ReportWarn` inline markup, callback handling, ham update, auto-learner ham signal, warning removal, admin-chat `/unwarn`, tests.
- Out of scope: changing public bot commands, detector scoring, warning strike rollback outside the detected-spam store.

## Options

1. Reuse existing unban callback with a warning-specific button.
   - Pros: minimal callback surface, already updates ham.
   - Cons: wrong side effects: unban/approve user, wrong labels.

2. Add a warning-specific callback prefix with ask/confirm states.
   - Pros: direct semantics, confirmation step, only ham and warning cleanup side effects.
   - Cons: small callback parser update and new handler.

3. Add a text reply command to mark warning as ham.
   - Pros: no inline callback changes.
   - Cons: less discoverable and does not satisfy button request.

## Decision

Use option 2. Add dedicated `Не спам` and confirmation callbacks for warning admin messages. Extract the clean warning message from the admin notification body, call `UpdateHam`/`LearnHam` only after confirmation, and clear all warning strikes for the user.
