# tgadmin standard launch plan

## Goal

Make the tgadmin Docker Compose setup the standard Docker launch path while keeping Railway/Railpack on the single-service `Dockerfile` deployment model.

## Steps

- [x] Add canonical `docker-compose.yml` based on `docker-compose-tgadmin.yml`.
- [x] Keep `docker-compose-tgadmin.yml` available for compatibility.
- [x] Add `railway.toml` with explicit `DOCKERFILE` builder and deploy settings.
- [x] Update README quick start and Docker Compose sections.
- [x] Update INSTALL guide to use the repository Compose file by default.
- [x] Validate Compose configs.
- [x] Archive the brainstorm and plan under `docs/plans/completed/`.

## Verification

- `docker compose -f docker-compose.yml config`
- `docker compose -f docker-compose-tgadmin.yml config`

No Go build/test is required because this change is deployment config and documentation only.
