# Delete Reply Command Brainstorm

## Scope

- Add `/del` as a reply command for configured superusers only.
- Delete the replied-to message, including bot-owned messages.
- Delete the command message after processing to keep chats clean.

## Direction

Route `/del` alongside superuser reply commands, but require `SuperUsers.IsSuper` directly instead of the broader `fromSuper` flag because linked-channel posts also pass that broader check.
