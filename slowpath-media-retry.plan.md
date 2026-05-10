# Slowpath Media Retry Plan

## Scope

- In scope: app/events media transform and slow-path invocation, tests for GIF/custom emoji/retry behavior.
- Out of scope: external ffmpeg/image decoding dependencies, database schema, public API changes.

## Steps

- [x] Extend internal message media metadata for animation thumbnails and custom emoji IDs.
- [x] Resolve custom emoji stickers through Telegram and use their thumbnail/file ID for vision.
- [x] Include GIF/animation thumbnail media in slow-path vision checks.
- [x] Add retry/backoff for retryable slow-path failures.
- [x] Add regression tests for GIF, custom emoji, and retry behavior.
- [x] Run gofmt and focused event tests.
- [x] Run build and broader relevant tests.
