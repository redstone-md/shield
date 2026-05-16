# Web UI Restyle — Design

- **Date:** 2026-05-16
- **Status:** Approved (pending spec review)
- **Scope:** Project B — full restyle of the Shield web UI. Branches from `dev` (which carries Project A, the editable settings UI).

## Problem

The web UI is stock Bootstrap 5.1 with ~150 lines of incidental custom CSS: a flat
grey theme, a 9-item top navbar, plain tables. It looks dated and generic. The
project is Minecraft-themed (`redstone-md/shield` — redstone, shield), and the UI
should reflect that identity while becoming a proper dark operations console.

## Goals

- Replace the stock Bootstrap look with a bespoke dark "ops-console" design system.
- Apply a tasteful Minecraft flavor: blocky shapes, a pixel display font, a
  Minecraft-mapped colour palette — without hurting readability.
- Convert the top navbar to a left sidebar (ops-console convention).
- Keep the no-build-step, server-rendered, embed.FS, HTMX architecture.

## Non-goals

- No backend/behaviour changes. Handlers, routes, data flow stay as they are.
- No build pipeline (no Tailwind/PostCSS/bundler). Hand-written CSS only.
- No new pages or features. This is a restyle of the existing 11 pages.
- Not a full Minecraft GUI emulation (no stone/dirt textures, no pixel-font body
  text) — the flavor is an accent layer, the console stays readable.

## Decisions (from brainstorming)

| Topic | Decision |
|---|---|
| Design system | Own hand-written CSS; **drop Bootstrap** (CSS + JS) |
| Theme | Single **dark** ops-console theme (no light theme, no toggle) |
| Interactivity | **Alpine.js** via CDN (sidebar collapse, dropdown, tabs, modals) |
| Layout | Top navbar → **left sidebar** |
| Minecraft flavor | Tasteful: pixel display font for brand/headings, blocky bevels, MC palette |
| Brand | "Shield" |
| Build step | None — CSS architecture: tokens + semantic component classes (single `styles.css`) |
| Kept as-is | HTMX, `embed.FS`, server-rendered Go templates, `bootstrap-icons` (icon font only) |

## Architecture & file structure

```
app/webapi/assets/
  styles.css                ← rewritten: tokens, reset, layout, components, utilities
  components/heads.html      ← rewritten <head> deps
  components/sidebar.html    ← was navbar.html: left sidebar nav
  components/dm_users.html   ← re-classed
  spam_check.html            ← all 11 pages re-classed + wrapped in sidebar+main layout
  manage_samples.html
  manage_users.html
  manage_dictionary.html
  detected_spam.html
  incidents.html
  incident_detail.html
  appeals.html
  feedback.html
  settings.html
  settings_edit.html
```

- `styles.css` is one organized file (no build): `:root` tokens → reset/base →
  layout (sidebar + main) → components → utilities.
- Each page stays a complete HTML document (existing Go-template pattern):
  `{{template "heads.html"}}` + `{{template "sidebar.html"}}` + content in `<main>`.
- `heads.html`: drop `bootstrap.min.css` and `bootstrap.bundle.min.js`; add Alpine.js
  (pinned CDN), keep `bootstrap-icons` (pinned), pin HTMX (currently unpinned), link
  `styles.css`, link the pixel fonts (Google Fonts).
- `components/navbar.html` is renamed to `sidebar.html`; every page's
  `{{template "navbar.html"}}` becomes `{{template "sidebar.html"}}`.

## Design tokens

Defined as CSS custom properties on `:root`.

- **Backgrounds (dark layers):** `--bg:#0e1116`, `--surface:#161b22`, `--elevated:#1c2230`.
- **Border:** `--border:#2a313c`.
- **Text:** `--text:#e6e9ee`, `--text-muted:#9aa4b2`.
- **Accent:** `--accent:#4f7cff` (lapis blue); a lighter hover variant.
- **Semantic (Minecraft-mapped):** `--spam:#d83a3a` (redstone), `--ham:#4caf50`
  (emerald), `--warn:#e0a526` (gold).
- **Typography:** body / tables / forms use `system-ui` for readability; a monospace
  stack for IDs and hashes; headings use `Pixelify Sans` (pixel but readable at
  heading sizes); the brand wordmark uses `Press Start 2P` (used sparingly). Pixel
  fonts loaded from Google Fonts (pinned).
- **Spacing:** 4px base scale — 4 / 8 / 12 / 16 / 24 / 32.
- **Radii:** blocky — 0–2px (Minecraft is square, not rounded).
- **Bevel:** buttons use a pixel bevel via `box-shadow` (light top-left edge, dark
  bottom-right edge), no `border-radius`. `image-rendering: pixelated` for pixel art.

## Layout shell

- **Sidebar:** fixed left, ~240px, `--surface` background. Brand + logo at top, the
  nav list (the existing 9 destinations, each with a `bootstrap-icons` glyph), and a
  user block at the bottom (Create Backup, Logout — the items currently in the navbar
  dropdown).
- **Mobile:** the sidebar moves off-canvas; a slim top bar with a hamburger appears.
  An Alpine `x-data` toggle drives an "open" class on the layout.
- **`<main>`:** to the right of the sidebar — a page header (title) and the content
  area in a max-width container.

## Components

Bootstrap classes are replaced by hand-written semantic classes:

| Bootstrap | New |
|---|---|
| `.btn .btn-primary` / `.btn-custom-blue` | `.btn`, `.btn-accent`, `.btn-ghost`, `.btn-danger` (beveled) |
| `.card` | `.card` (surface, 1px border, blocky) |
| `.table .table-striped` / `.custom-table-header` | `.table` (dark, zebra rows, styled header) |
| `.form-control` / `.form-select` / `.form-check` | `.input`, `.select`, `.checkbox`, `.field` wrapper |
| `.alert-*` | `.alert`, `.alert-danger` / `.alert-success` / `.alert-warn` |
| `.badge` `bg-danger` / `bg-success` | `.badge`, `.badge-spam` / `.badge-ham` / `.badge-warn` |
| `.nav-tabs` (feedback page) | `.tabs` — Alpine `x-data` tracks the active tab |
| `.dropdown` (user menu) | Alpine `x-data` dropdown |
| Bootstrap modals (appeals / incident detail, if any) | `.modal` — Alpine-driven |
| `.container` / `.row` / `.col-*` grid | no 12-column clone — targeted layout classes: `.form-row` (label + control), `.card-grid`, flex utilities |

The Bootstrap 12-column grid is removed; the admin pages need only form rows,
tables, stacked cards and a simple card grid, which targeted classes cover.

## Interactivity (Alpine.js)

Alpine handles what Bootstrap JS used to: sidebar mobile toggle, the user dropdown,
the feedback page tabs, and any modals. HTMX stays the mechanism for server
interactions.

**HTMX + Alpine interplay:** HTMX swaps DOM fragments; Alpine components inside a
swapped fragment must be initialized. The plan wires an `htmx:afterSwap` listener
that calls `Alpine.initTree` on the swapped node (or uses `hx-on`). This is a named
implementation task, not an afterthought.

## Testing

- **e2e-ui Playwright tests** (`e2e-ui/e2e_test.go`) select on Bootstrap-specific
  hooks — `data-bs-toggle`, `data-bs-target`, `.navbar`, heading text. Dropping
  Bootstrap breaks these selectors. Updating the e2e tests to the new markup is a
  mandatory part of the work, not a follow-up.
- **Per page:** the template must still parse (the global `template.Must` parses all
  assets at init — a broken template fails every webapi test) and the handler must
  still return 200.
- **Visual correctness** is verified manually; CI checks structure and text only.

## Risks

- All 11 pages change at once — a half-converted UI (Bootstrap + new classes mixed)
  looks broken, so the restyle lands as a single branch and merges whole.
- HTMX + Alpine initialization on swapped content (covered above).
- Bootstrap JS behaviors (collapse, dropdown, tabs, modals) must each be
  re-verified after re-implementation on Alpine.
- Wide data tables (`detected_spam`, `incidents`) carry custom column sizing in the
  current CSS; the new table styles must keep them usable.

## Out of scope (separate efforts)

- Light theme / theme toggle.
- Full Minecraft GUI emulation (textures, pixel-font body text).
- New pages, new features, backend changes.
