# tgadmin standard launch brainstorm

## Scope

In scope:
- Make the tgadmin Docker Compose setup the default Docker Compose launch path.
- Keep Railway/Railpack deployment as a single `Dockerfile`-built service.
- Document the difference between local/VPS Compose and Railway deployment.

Out of scope:
- Changing application runtime behavior.
- Merging `cloudflared` into the `tg-spam` Docker image.
- Adding new dependencies or database changes.

## Options

1. Rename `docker-compose-tgadmin.yml` to `docker-compose.yml` and document it as the default.
   - Pro: matches standard `docker compose up -d` behavior.
   - Pro: minimal operational surprise for Docker users.
   - Con: need to preserve old filename for users already calling it explicitly.

2. Keep only `docker-compose-tgadmin.yml` and update docs to always pass `-f`.
   - Pro: no duplicate Compose files.
   - Con: less standard; not what users expect from a default Compose setup.

3. Build a Railway-specific multi-process image with `tg-spam` and `cloudflared`.
   - Pro: superficially closer to the Compose topology.
   - Con: poor container design, duplicates Railway ingress, complicates restarts and shutdown.

## Decision

Use option 1. Add `docker-compose.yml` as the canonical tgadmin Compose setup, keep `docker-compose-tgadmin.yml` as a compatibility copy, and add Railway config that explicitly builds the single app container from `Dockerfile`.

## Risks

- Duplicate Compose files can drift. Keep the compatibility file identical.
- Railway healthcheck requires the web server to be enabled via environment variables.
- Existing docs may still mention `docker run` or minimal Compose as the primary path.
