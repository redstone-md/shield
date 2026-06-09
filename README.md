# Shield

Shield is a self-hosted Telegram moderation and anti-spam bot. It watches group messages, scores them through fast local checks, optionally escalates harder cases to LLM or vision providers, and applies moderation policy such as allow, delete, restrict, warn, or ban.

> Note: the runtime package, binary, and some internal paths still use the legacy `tg-spam` name. This README uses **Shield** for the repository/product and `tg-spam` where it is the current executable name.

<div align="center">
  <img class="logo" src="https://github.com/redstone-md/shield/raw/master/site/tg-spam-bg.png" width="400px" alt="TG-Spam | Spam Hunter"/>
</div>

<div align="center">

[![build](https://github.com/redstone-md/shield/actions/workflows/ci.yml/badge.svg)](https://github.com/redstone-md/shield/actions/workflows/ci.yml)&nbsp;[![Go Report Card](https://goreportcard.com/badge/github.com/redstone-md/shield)](https://goreportcard.com/report/github.com/redstone-md/shield)

<img src="site/powered_by.png" alt="Powered by redstone.md" width="150"/>

</div>

## What Shield does

- Moderates Telegram groups with automatic spam detection and enforcement.
- Uses fast-path checks for sample similarity, Bayesian scoring, stop words, CAS lookups, emoji count, links, mentions, media-only messages, forwarded messages, keyboards, giveaways, duplicate messages, abnormal spacing, and mixed-language words.
- Supports OpenAI and Gemini checks for LLM-assisted moderation, including veto/consensus modes, history-aware checks, custom prompts, and short-message handling.
- Supports a slow-path moderation layer for image/LLM review. When OpenAI or Gemini tokens are configured, the bot automatically registers them as both text and vision providers. Image messages not already flagged by the fast path are downloaded and sent for vision analysis. No additional flags required.
- Provides admin workflows for reporting spam, confirming or reversing actions, warning users, soft bans, dry runs, training mode, and aggressive cleanup.
- Stores moderation data in SQLite by default, with PostgreSQL support for larger deployments.
- Can run a web server/API for administration, settings, review flows, and operational visibility.
- Supports custom Lua plugins for project-specific spam patterns without changing Go code.

## Quick start

Create a Telegram bot with BotFather, add it to your group as an admin, then run the standard tgadmin Compose setup:

```bash
cp .env.example .env
$EDITOR .env
docker compose up -d
```

The default `docker-compose.yml` starts `tg-spam` with persistent data in `./var/tg-spam`, logs in `./logs`, and a `cloudflared-tgadmin` tunnel sidecar. If you do not use Cloudflare Tunnel, remove or disable the `cloudflared-tgadmin` service before starting.

For non-technical setup instructions, see [INSTALL.md](/INSTALL.md).

## Installation

- **Docker** (primary method): image available at [ghcr.io/redstone-md/shield](https://github.com/redstone-md/shield/pkgs/container/shield).
- **Binary releases**: [releases page](https://github.com/redstone-md/shield/releases/latest).
- **From source**: `make build` produces `.bin/tg-spam`.
- **macOS**: `brew tap redstone-md/apps && brew install redstone-md/apps/tg-spam`.

## Configuration

The bot is configured through command-line flags or environment variables. Out of the box, reasonable defaults allow running with just two mandatory parameters.

### Required parameters

| Environment variable | Flag | Purpose |
|---|---|---|
| `TELEGRAM_TOKEN` | `--telegram.token` | Telegram bot token from BotFather |
| `TELEGRAM_GROUP` | `--telegram.group` | Group username or numeric group ID (single group, legacy) |
| `TELEGRAM_GROUPS` | `--telegram.groups` | Comma-separated group IDs/usernames (overrides `TELEGRAM_GROUP` when set) |

### Common optional settings

| Environment variable | Flag | Purpose |
|---|---|---|
| `DB` | `--db` | SQLite file or PostgreSQL URL |
| `FILES_DYNAMIC` | `--files.dynamic` | Dynamic data directory |
| `ADMIN_GROUP` | `--admin.group` | Admin chat/group for reports and moderation controls |
| `OPENAI_TOKEN` | `--openai.token` | Enable OpenAI text and vision checks |
| `OPENAI_API_BASE` | `--openai.apibase` | Custom OpenAI-compatible endpoint |
| `GEMINI_TOKEN` | `--gemini.token` | Enable Gemini text and vision checks |
| `FILES_DYNAMIC` | `--files.dynamic` | Dynamic data directory; put `prompt-override.md` here to override the slow-path system prompt |
| `LLM_CONSENSUS` | `--llm.consensus` | `any` or `all` when multiple LLMs are eligible |
| `LLM_MIN_INPUT_CHARS` | `--llm.min-input-chars` | Minimum text length for automatic LLM checks; default `5` |
| `REPORT_ENABLED` | `--report.enabled` | Enable user `/report` flow |
| `SERVER_ENABLED` | `--server.enabled` | Enable HTTP server/API/UI |
| `SERVER_LISTEN` | `--server.listen` | HTTP listen address, default `:8080` |
| `LUA_PLUGINS_ENABLED` | `--lua-plugins.enabled` | Enable Lua plugin checks |

Run the binary with `--help` to see the full generated option list.

## Moderation pipeline

Shield separates the moderation flow into stages:

1. **Ingest**: Telegram updates are normalized and enqueued.
2. **Fast path**: Local spam checks run first (similarity, classifier, stop words, CAS, emoji, meta, duplicates, spacing, multi-lang, Lua plugins). Latency and usage metrics are recorded.
3. **Slow path** (automatic when LLM tokens are set): if the message contains an image not already flagged by the fast path, the image is downloaded and sent to the configured vision provider. Text-only LLM escalation is also available for ambiguous cases.
4. **Policy**: decides the final action using the message, fast/slow check results, rule-set version, strike history, soft-ban mode, dry-run mode, and superuser status.
5. **Enforce**: the selected action (allow, delete, restrict, warn, ban) is applied and audit/incident state is recorded.

Rule sets cover meta checks, duplicates, abnormal spacing, moderation behavior, reports, OpenAI, Gemini, policy profile, and the slow-path feature flag.

## Spam detection modules

### Message Analysis (Bayesian classifier)

The main detection module. Uses spam and ham samples with a Bayes classifier. Active only if both ham and spam samples are present in the database. Minimum spam probability is controlled by `--min-probability` (default 50).

### Spam message similarity

Compares messages against known samples. Marks as spam if similarity exceeds `--similarity-threshold` (default 0.5). Set to 1 to disable.

### Stop words

Checks messages against a curated stop-word list stored in the database. Supports substring matching (`buy now` matches any message containing "buy now") and exact matching (`=buy now` matches only if the entire message equals "buy now"). Case-insensitive.

### Combot Anti-Spam System (CAS)

Enabled by default. Cross-references users with the external CAS database. Disable with `--cas.api=""`. Custom User-Agent: `--cas.user-agent`.

### OpenAI integration

Setting `--openai.token` enables OpenAI for both text and vision analysis.

**Text checks:**
- Runs as the final check in the detection pipeline.
- Without `--openai.veto`: OpenAI is called only if preceding checks did not flag the message as spam. This increases spam catch rate.
- With `--openai.veto`: OpenAI is called *only* when other checks flag the message as spam. The message is classified as spam only if OpenAI agrees. This reduces false positives.
- `--openai.history-size` provides conversation context (5-10 messages recommended).
- `--openai.check-short-messages` enables checking messages shorter than `--min-msg-len`.
- `--openai.custom-prompt` adds custom spam patterns (repeatable).
- `--openai.reasoning-effort` controls thinking mode for supported models (`none`, `low`, `medium`, `high`; default `none`).
- `--openai.model` changes the model (default `gpt-4o-mini`).

**Vision checks (automatic):**
- When OpenAI is configured and a message contains an image not already flagged by the fast path, the image is downloaded from Telegram and sent to OpenAI's vision API for analysis.
- Uses the same circuit breaker and budget tracking as text checks.
- No additional configuration required.

### Google Gemini integration

Setting `--gemini.token` enables Gemini as an alternative or additional LLM. Supports the same veto, history, custom prompts, and short-message options as OpenAI. Default model: `gemma-4-31b-it`.

Vision analysis works identically to OpenAI: images are automatically sent to Gemini's multimodal API when configured.

### LLM consensus

When multiple LLM providers are eligible for the same message, Shield resolves their results with `--llm.consensus`:

- `any` (default): if any eligible LLM disagrees with the base decision, the base decision flips.
- `all`: all eligible LLMs must agree before the base decision flips.
- Each request is subject to `--llm.request-timeout` (default 30s).
- Automatic LLM checks skip text shorter than `--llm.min-input-chars` (default 5). Use `--openai.check-short-messages=false` and `--gemini.check-short-messages=false` to avoid LLM checks for short messages that the base detector allowed; short flagged spam can still be veto-checked above the minimum length. Forced report reviews bypass the minimum length.

### Custom slow-path system prompt

To override the built-in slow-path text system prompt, create `prompt-override.md` in the dynamic data directory. With the Docker quick start this means `./data/prompt-override.md`, mounted as `/srv/data/prompt-override.md` in the container. The same file is used for OpenAI and Gemini slow-path text checks.

Explicit provider prompts have higher priority than the file: `--openai.prompt` or `OPENAI_PROMPT` wins for OpenAI, and `--gemini.prompt` or `GEMINI_PROMPT` wins for Gemini. If neither an explicit provider prompt nor `prompt-override.md` is present, Shield uses the built-in default prompt.

The custom system prompt must preserve the response contract expected by Shield:

- Return only valid JSON, with no Markdown fences or explanatory text outside JSON.
- Use exactly these fields: `spam` as boolean, `reason` as short string, and `confidence` as integer from 1 to 100.
- Keep the same decision meaning: mark spam only when confidence is above 80.
- Write `reason` in the language your moderators expect; the built-in prompt uses Russian.
- Include your local spam priorities, such as crypto exchange ads, illegal work, repeated ads, fraud, abuse, drugs, suspicious links, QR-code scams, and emoji spam.
- Do not ask the model to reveal hidden reasoning; keep the reason concise and operator-readable.

Minimal example:

```md
Return only JSON: {"spam":true/false,"reason":"why","confidence":1-100}.
Spam only if confidence > 80.
This is a Russian-speaking Telegram chat, write reason in Russian.
Prioritize crypto exchange ads, illegal work, repeated ads, fraud, suspicious links, drugs, abuse, and emoji spam.
```

### Emoji count

Messages with more than `--max-emoji` emojis (default 2) are flagged as spam. Set to -1 to disable, 0 to flag any emoji.

### Meta checks

| Check | Flag | Behavior |
|---|---|---|
| Image only | `--meta.image-only` | Flag images with text shorter than `--min-msg-len` |
| Links limit | `--meta.links-limit` | Flag if links exceed limit (default -1 = disabled) |
| Links only | `--meta.links-only` | Flag messages containing links but no text |
| Mentions limit | `--meta.mentions-limit` | Flag if mentions exceed limit (default -1 = disabled) |
| Video only | `--meta.video-only` | Flag videos with text shorter than `--min-msg-len` |
| Audio only | `--meta.audio-only` | Flag audio files with text shorter than `--min-msg-len` |
| Contact only | `--meta.contact-only` | Flag shared contacts with no text |
| Forward | `--meta.forward` | Flag forwarded messages |
| Keyboard | `--meta.keyboard` | Flag messages with inline keyboards |
| Username symbols | `--meta.username-symbols` | Flag usernames containing prohibited characters |
| Giveaway | `--meta.giveaway` | Flag giveaway messages |

### Multi-language words

Detects words mixing characters from multiple languages. Enable with `--multi-lang=N` (default 0 = disabled).

### Duplicate message detection

Tracks messages per user and flags identical repeats within a time window. Runs for all users including approved users.

- `--duplicates.threshold` (default 0 = disabled): number of identical messages to trigger.
- `--duplicates.window` (default 1h): tracking time window.

### Abnormal spacing

Detects spacing tricks used to break up spam words. Enable with `--space.enabled`.

- `--space.ratio` (default 0.3): space-to-character ratio threshold.
- `--space.short-ratio` (default 0.7): short-word ratio threshold.
- `--space.short-word` (default 3): max length for "short" words.
- `--space.min-words` (default 5): minimum words to trigger the check.

### Lua plugins

Custom spam detection logic without editing Go code. Enable with `--lua-plugins.enabled`, point `--lua-plugins.plugins-dir` at a directory of `.lua` scripts, and optionally restrict enabled plugins with `--lua-plugins.enabled-plugins`. Dynamic reload: `--lua-plugins.dynamic-reload`.

Each plugin exposes a `check(request)` function returning `(isSpam bool, details string)`. Helper functions: `count_substring`, `match_regex`, `contains_any`, `to_lower`, `to_upper`, `trim`, `split`, `join`, `starts_with`, `ends_with`.

Example plugins: [_examples/lua_plugins](https://github.com/redstone-md/shield/tree/master/_examples/lua_plugins).

## Persistence and migration

By default, Shield uses SQLite and stores data in the dynamic data directory. In Docker, mount `/srv/data` so the database, learned samples, user approvals, incidents, reports, and moderation history survive restarts.

PostgreSQL is supported by setting `--db` to a PostgreSQL URL.

### Database migration from text files (v1.16.0+)

Starting from v1.16.0, all data (spam/ham samples, stop words, excluded tokens) is stored in the database. The `--convert` parameter controls migration:

- `enabled` (default): migrates on startup if needed, then continues.
- `disabled`: skips migration, requires data already in the database.
- `only`: migrates and exits immediately.

On first startup after upgrading, the bot automatically migrates text files, renames them to `*.loaded`, and continues. Renaming `.loaded` files back to `.txt` triggers a fresh migration.

### Automatic backup on version upgrade

When a version upgrade is detected, Shield creates a timestamped database backup before applying changes. Controlled by `--max-backups` (default 10). Set to 0 to disable.

## Admin and user reporting

### Admin chat

Setting `--admin.group` enables an admin chat where the bot reports detected spam. Admins can:
- **Confirm ban**: click a button on the spam report.
- **Unban**: reverse a ban and add the message to ham samples.
- **`/spam`** or **`spam`** reply: mark a message as spam, ban the user, add to spam samples.
- **`/ban`** or **`ban`** reply: ban without adding to samples.
- **`/warn`** or **`warn`** reply: delete the message and send a warning.

**Aggressive cleanup** (`--aggressive-cleanup`): when admins use `/spam` or `/ban`, delete all recent messages from the banned user (up to `--aggressive-cleanup-limit`, default 100).

**Linked channel**: if the group is linked to a Telegram channel, the channel automatically receives superuser privileges without extra configuration.

### User spam reporting

Regular users can reply to suspicious messages with `/report`. Enable with `--report.enabled`.

- `--report.threshold` (default 2): reports needed for admin notification.
- `--report.auto-ban-threshold`: auto-ban without admin approval (0 = disabled, must be >= threshold).
- `--report.rate-limit` / `--report.rate-period`: rate limiting to prevent abuse.
- Only approved users can submit reports.

### Dynamic sample updates

Super-users forwarding spam to the admin chat (or replying `/spam`) adds it to spam samples automatically. Unbanning adds the message to ham samples. The bot learns new patterns on the fly.

### Appeal flow

When the bot warns or bans a user it posts a group-chat message carrying an
**"Обжаловать"** (Appeal) inline button. A ban now posts its own group message
(previously only the admin chat was notified); both the warn and the ban
message auto-delete after `WarnDeleteDuration`, so the appeal button is
available for that window.

Tapping the button opens the bot DM and files a one-tap appeal (no reason
text). The appeal is sent to the admin chat with **Принять / Отклонить**
buttons. Accepting unbans the user, clears all of their warning strikes and
DMs them the outcome; rejecting closes the incident and DMs the user. Each
incident accepts a single appeal — a moderator decision is final.

The same accept/reject behavior backs the web `/appeals` admin UI, so an
appeal resolved on the website unbans and notifies the user identically.

## Web server and UI

Enable with `--server.enabled`. Listen address: `--server.listen` (default `:8080`).

Protected by basic auth (user `tg-spam`). Default password is auto-generated and printed on startup. Set custom password with `--server.auth` or a bcrypt hash with `--server.auth-hash`.

### API endpoints

| Method | Path | Purpose |
|---|---|---|
| GET | `/ping` | Health check |
| POST | `/check` | Check message for spam (JSON body: `msg`, `user_id`, `user_name`) |
| GET | `/check/{user_id}` | Detected spammer status |
| POST | `/update/spam` | Add spam sample |
| POST | `/update/ham` | Add ham sample |
| POST | `/delete/spam` | Remove spam sample |
| POST | `/delete/ham` | Remove ham sample |
| POST | `/users/add` | Approve user |
| POST | `/users/delete` | Remove approved user |
| GET | `/users` | List approved users |
| GET | `/samples` | List spam/ham samples |
| PUT | `/samples` | Reload dynamic samples |
| GET | `/settings` | Current bot settings |
| GET | `/dm-users` | Recent DM senders |

For request examples, see [webapp.rest](https://github.com/redstone-md/shield/blob/master/webapp.rest).

### Web UI

The web UI provides management interfaces:

- **Message Checker**: test messages for spam detection in real-time.
- **Manage Samples**: add, view, and delete spam/ham training samples.
- **Dictionary Management**: manage stop phrases and ignored tokens.
- **Manage Users**: view and control the approved users list.
- **Detected Spam**: browse detected spam history.
- **Settings**: configure bot parameters, super-users, and find your Telegram user ID.
- **Edit Settings**: at `/settings/edit`, change the detection and LLM tuning (thresholds, per-check toggles, LLM mode/veto/consensus, models, system and vision prompts) directly in the browser. Saving stores a new versioned rule set and applies the change live without a restart. A setting that is also pinned by an environment variable shows an `env-pinned` warning badge, since the env value overrides the stored rule set on the next restart — remove it from the environment to manage that setting from the UI.

<details markdown>
  <summary>Screenshots</summary>

![msg-checker](https://github.com/redstone-md/shield/raw/master/site/docs/msg-checker.png)

![manage-samples](https://github.com/redstone-md/shield/raw/master/site/docs/manage-samples.png)

![manage-users](https://github.com/redstone-md/shield/raw/master/site/docs/manage-users.png)
</details>

## Docker compose

The standard Docker Compose entry point is [`docker-compose.yml`](docker-compose.yml):

```bash
cp .env.example .env
$EDITOR .env
docker compose up -d
```

Set at least `TELEGRAM_TOKEN`, `TELEGRAM_GROUP` (or `TELEGRAM_GROUPS`), and `SERVER_ENABLED=true` in `.env` for the tgadmin web UI. For the bundled Cloudflare Tunnel sidecar, add `CLOUDFLARED_TOKEN=<tunnel-token>` to `.env`; Compose interpolates `${CLOUDFLARED_TOKEN}` before service-level `env_file` values are loaded.

[`docker-compose-tgadmin.yml`](docker-compose-tgadmin.yml) is kept as a compatibility alias for the same tgadmin topology.

## Docker compose examples

### Minimal

```yaml
services:
  tg-spam:
    image: ghcr.io/redstone-md/shield:latest
    restart: always
    environment:
      - TELEGRAM_TOKEN=your-bot-token
      - TELEGRAM_GROUP=your-group-name
    volumes:
      - ./data:/srv/data
```

### With admin chat, reporting, and logging

```yaml
services:
  tg-spam:
    image: ghcr.io/redstone-md/shield:latest
    hostname: tg-spam
    restart: always
    container_name: tg-spam
    user: "1000:1000"
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "5"
    environment:
      - TZ=America/Chicago
      - TELEGRAM_TOKEN=your-bot-token
      - TELEGRAM_GROUP=example_chat
      - ADMIN_GROUP=-403767890
      - LOGGER_ENABLED=true
      - LOGGER_FILE=/srv/log/tg-spam.log
      - LOGGER_MAX_SIZE=5M
      - NO_SPAM_REPLY=true
      - REPORT_ENABLED=true
      - REPORT_THRESHOLD=2
    volumes:
      - ./data:/srv/data
      - ./logs:/srv/log
    command: --super=user1 --super=user2
```

### With web server and reverse proxy

See [docker-compose.yml](docker-compose.yml) for the standard tgadmin setup with web UI and Cloudflare Tunnel, or [docker-compose-with-server.yml](docker-compose-with-server.yml) for the older web-server compose example.

### With PostgreSQL

See [docker-compose-with-psql.yml](docker-compose-with-psql.yml).

## Railway

Railway deploys this repository as a single `tg-spam` service built from [`Dockerfile`](Dockerfile). The checked-in [`railway.toml`](railway.toml) pins that behavior explicitly.

Dokploy/Railpack deployments use [`railpack.json`](railpack.json), which pins the Go entrypoint to `./app`. This is required because the repository root has `go.mod`, while the executable package lives under `app/` rather than the root or `cmd/`.

Set the required Railway variables in the service settings:

```env
TELEGRAM_TOKEN=<bot-token>
TELEGRAM_GROUP=<group-name-or-id>
TELEGRAM_GROUPS=<comma-separated-groups>
SERVER_ENABLED=true
SERVER_LISTEN=:8080
FILES_DYNAMIC=/srv/data
```

Do not run the Compose `cloudflared-tgadmin` sidecar on Railway; Railway provides its own public ingress/domain for the web UI.

## Multiple groups

Set `TELEGRAM_GROUPS` to a comma-separated list of group IDs or usernames to monitor multiple groups with a single bot instance. Bans and unbans apply to all managed groups; message deletions target the originating chat. Strikes are global per-user across all groups.

```env
TELEGRAM_GROUP=legacy_single_group
TELEGRAM_GROUPS=group1,-1001234567890,group3
```

`TELEGRAM_GROUPS` overrides `TELEGRAM_GROUP` when set. For backward compatibility, a single `TELEGRAM_GROUP` value still works.

## Using as a library

Import `github.com/redstone-md/shield/lib/tgspam` and use the `Detector` directly:

```go
package main

import (
    "fmt"
    "io"
    "net/http"
    "strings"

    "github.com/redstone-md/shield/lib/spamcheck"
    "github.com/redstone-md/shield/lib/tgspam"
)

func main() {
    detector := tgspam.NewDetector(tgspam.Config{
        MaxAllowedEmoji:  5,
        MinMsgLen:        10,
        FirstMessageOnly: true,
        CasAPI:           "https://cas.chat",
        HTTPClient:       &http.Client{},
    })

    stopWords := strings.NewReader("\"word1\"\n\"word2\"\n\"hello world\"")
    res, err := detector.LoadStopWords(stopWords)
    if err != nil {
        panic(err)
    }
    fmt.Println("loaded", res.StopWords, "stop words")

    spam := strings.NewReader("spam sample 1\nspam sample 2")
    ham := strings.NewReader("ham sample 1\nham sample 2")
    excluded := strings.NewReader("\"the\", \"a\", \"an\"")
    res, err = detector.LoadSamples(excluded, []io.Reader{spam}, []io.Reader{ham})
    if err != nil {
        panic(err)
    }
    fmt.Println("loaded", res.SpamSamples, "spam,", res.HamSamples, "ham")

    isSpam, info := detector.Check(spamcheck.Request{
        Msg: "This is a test message", UserID: "user1", UserName: "John Doe",
    })
    fmt.Println("spam:", isSpam, "info:", info)
}
```

For API docs, see [pkg.go.dev](https://pkg.go.dev/github.com/redstone-md/shield/lib). For complete examples, see [_examples/](https://github.com/redstone-md/shield/tree/master/_examples/).

## Getting spam samples from CAS

The provided [`cas-export.sh`](https://raw.githubusercontent.com/redstone-md/shield/master/cas-export.sh) script downloads spam samples from the CAS API. Requires `jq` and `curl`. Use the output as a base for your spam samples, not as-is — every group has different spam patterns.

```bash
curl -s https://raw.githubusercontent.com/redstone-md/shield/master/cas-export.sh > cas-export.sh
chmod +x cas-export.sh
./cas-export.sh
```

## Updating samples from a remote git repository

A utility container is provided for automated sample updates from git. See [updater/README.md](https://github.com/redstone-md/shield/tree/master/updater/README.md).

## Development

```bash
make build          # build .bin/tg-spam
make test           # race-enabled tests with coverage summary
make race_test      # race test suite
make docker         # build local Docker image
make e2e-ui-setup   # install Playwright browser dependencies
make e2e-ui         # run headless UI e2e tests
make e2e-ui-debug   # run visible-browser UI e2e tests
```

The Go module path is `github.com/redstone-md/shield`.

## Repository map

| Path | Purpose |
|---|---|
| `app/` | Runtime assembly, Telegram listener, web server, policy, rules, storage, slow path |
| `lib/tgspam/` | Core spam detector and reusable checks |
| `lib/textnorm/` | Text normalization pipeline |
| `lib/spamcheck/` | Shared request/response types |
| `app/slowpath/` | Slow-path LLM/vision engine with circuit breakers |
| `app/storage/` | SQLite/Postgres persistence layer (20+ store types) |
| `app/controlplane/` | Service layer: workspace, tenant, rules, dictionary, onboarding |
| `app/webapi/` | Server-rendered web UI (HTMX) + JSON API |
| `app/events/` | Telegram ingestion, admin handlers, moderation pipeline |
| `app/policy/` | Policy decision engine with profiles |
| `app/audit/` | Incident management and appeal resolution |
| `app/feedback/` | Feedback labeling, review candidates, knowledge snapshots |
| `data/` | Preset samples and dictionaries |
| `site/` | MkDocs documentation site |
| `docs/` | Architecture, ADRs, roadmap, plans |
| `_examples/` | Example integrations |
| `e2e-ui/` | Playwright end-to-end UI tests |
