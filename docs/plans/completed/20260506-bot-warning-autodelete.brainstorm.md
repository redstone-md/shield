# Bot Warning Autodelete Brainstorm

## Problem

Automatic moderation warnings are deleted after `WarnDeleteDuration`, but manual admin `/warn` messages (`warning from <admin>`) are not deleted because the direct warn path omits the warning delete duration.

## Options

- Add a separate deletion goroutine in `admin.DirectWarnReport`. Rejected: duplicates executor behavior.
- Reuse `ActionExecutor.WarnUser` delete scheduling by passing `warnDelTime`. Chosen: same mechanism as automatic warnings.

## Scope

In scope: admin direct `/warn` messages sent to the primary chat.

Out of scope: admin-chat notifications and report review messages with inline keyboards.
