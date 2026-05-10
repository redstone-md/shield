# Slowpath Media Retry Brainstorm

## Scope

- Add slow-path vision coverage for premium/custom emoji and GIF animations.
- Use Telegram-provided thumbnails as the first-frame image source where available.
- Add retry backoff for transient slow-path provider failures.
- Keep moderation side effects unchanged.

## Options

1. Decode first frames locally.
   - Pros: true first-frame extraction.
   - Cons: needs new decoding dependencies or external ffmpeg, larger runtime surface.

2. Use Telegram thumbnails as first-frame snapshots.
   - Pros: no new dependencies, available through existing Bot API metadata, safe for animations/GIFs.
   - Cons: thumbnail is Telegram's representative frame, not guaranteed exact frame zero.

3. Send raw GIF/video to vision.
   - Pros: simple file download.
   - Cons: current downloader and slow-path contract are image-oriented.

## Decision

Use option 2. Treat Telegram thumbnails as the first-frame image source for GIF animations, animated/video stickers, and custom premium emoji stickers. Add a generic retry wrapper around slow-path checks for retryable provider errors with capped exponential backoff.
