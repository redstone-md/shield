# Web UI Restyle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the stock Bootstrap web UI of the Shield bot with a bespoke dark "ops-console" design system that carries a tasteful Minecraft flavor, converting the top navbar to a left sidebar.

**Architecture:** Hand-written CSS design system (tokens + semantic component classes in one `styles.css`, no build step). Bootstrap CSS/JS dropped. Alpine.js (CDN) replaces Bootstrap JS for the mobile sidebar toggle, the feedback/settings tabs, and the settings collapse panel. HTMX, `embed.FS`, server-rendered Go templates and the `bootstrap-icons` font are kept. The top navbar component becomes a left sidebar; every page is re-classed and wrapped in a `sidebar + main` shell.

**Tech Stack:** Go 1.24 `html/template` via `embed.FS`, HTMX 2.x, Alpine.js 3.x (CDN), hand-written CSS, `bootstrap-icons` font, Playwright (e2e).

**Spec:** `docs/superpowers/specs/2026-05-16-ui-restyle-design.md`

---

## Notes for the implementer

- This is a **UI restyle** — no Go behaviour, route, or data-flow changes (except adding one static-file mapping in `routes.go`).
- The pages are server-rendered `html/template` documents. All templates are parsed at package init by `template.Must(...ParseFS(...))` in `app/webapi/webapi.go:42`. **A single broken template panics package init and fails every `app/webapi` test** — so `go test ./app/webapi/` is the parse gate for every page task.
- The template name equals the file's base name (e.g. `{{template "sidebar.html"}}` renders `assets/components/sidebar.html`).
- Page conversion tasks are refactors verified by the existing `app/webapi` handler-test suite (template-parses + handler returns 200). The genuine test changes live in Task 16 (webapi unit tests) and Task 17 (e2e tests).
- Comments in code stay lowercase (project convention). Do not add history/progress comments.
- Run commands from the repo root: `C:\Users\nevermore\Documents\bprojects\redstone-md\shield`.
- All work happens on the existing branch `feat/ui-restyle`.

---

## Reference (used by every page task)

### R1 — Page shell skeleton

Every full-page template (the 11 pages, **not** the `{{define}}` fragments) follows this exact skeleton. Only `<title>`, the `page-title` text, and the `.content` body differ per page:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>PAGE TITLE - Shield</title>
    {{template "heads.html"}}
</head>
<body x-data="{ navOpen: false }">
{{template "sidebar.html"}}
<div class="scrim" x-show="navOpen" @click="navOpen=false" x-cloak></div>
<main class="main">
    <header class="topbar">
        <button class="topbar__burger" @click="navOpen=!navOpen" aria-label="Toggle navigation"><i class="bi bi-list"></i></button>
        <h2 class="page-title">PAGE TITLE</h2>
    </header>
    <div class="content">
        <!-- page body -->
    </div>
</main>
</body>
</html>
```

- Page titles stay in an `<h2 class="page-title">` element (e2e selects on `h2:has-text(...)`).
- `{{template "navbar.html"}}` is gone everywhere; it is replaced by `{{template "sidebar.html"}}`.
- The `{{define "..."}}` HTMX fragment sub-templates at the bottom of pages are **not** wrapped in the shell — they are swapped into the DOM. Only their classes change.

### R2 — Bootstrap → design-system class mapping

Apply this mapping to every page. Anything not listed is removed.

| Bootstrap (old) | New |
|---|---|
| `container`, `container-fluid`, `container mt-4` | removed — the shell's `.content` provides the wrapper |
| `row` + two `col-md-6` | parent `.grid-2`, children become plain `<div>` |
| `row` + `col-md-8` / `col-md-4` | parent `.grid-2-1`, children plain `<div>` |
| `row g-2 align-items-end` (filter forms) | `.filter-bar` |
| `col-*`, `col-md-*`, `col-12`, `offset-md-3` | removed (child of a grid/flex parent) |
| `btn btn-primary` | `btn btn-accent` |
| `btn btn-danger` | `btn btn-danger` (kept) |
| `btn btn-success` | `btn btn-ham` |
| `btn btn-warning` | `btn btn-warn` |
| `btn btn-secondary`, `btn btn-outline-*`, `btn-custom-blue`, `btn-custom-blue-outline` | `btn btn-ghost` |
| `btn-sm` | `btn-sm` (kept) |
| `card`, `card-header`, `card-body`, `card-text` | kept (restyled by CSS) |
| `table`, `table-sm` | `table` |
| `table-striped`, `table-hover` | removed (CSS handles zebra + hover) |
| `table-responsive` | wrap the `<table>` in `<div class="table-wrap">` |
| `thead class="custom-table-header"` | `<thead>` (CSS styles `thead`) |
| `form-control`, `form-control-sm` | `input` |
| `form-select`, `form-select-sm` | `select` |
| `form-label` | `form-label` (kept) |
| `form-check` wrapper | removed |
| `form-check-input` | `checkbox` |
| `input-group` | `.flex .gap-2 .wrap` |
| `alert` | `alert` (kept) |
| `alert-danger` / `alert-success` / `alert-info` | `alert-danger` / `alert-success` / `alert-info` (kept) |
| `alert-light` | `alert` |
| `d-none` | `hidden` |
| `d-none d-md-block` | `only-desktop` |
| `d-md-none` | `only-mobile` |
| `badge bg-danger` | `badge badge-spam` |
| `badge bg-success` | `badge badge-ham` |
| `badge bg-warning` | `badge badge-warn` |
| `badge bg-info`, `badge bg-primary`, `badge bg-purple` | `badge badge-info` |
| `badge bg-secondary`, `badge bg-light text-dark` | `badge badge-muted` |
| `d-flex` | `flex` |
| `d-grid` / `d-grid gap-2` | `flex flex-col gap-2` |
| `d-inline` / `d-inline-block` | removed |
| `justify-content-between` | `between` |
| `justify-content-end` | `end` |
| `justify-content-center` | removed (use `.content` width or `.center`) |
| `align-items-center` / `align-items-end` | `center` |
| `flex-wrap` | `wrap` |
| `gap-1` / `gap-2` | `gap-1` / `gap-2` (kept) |
| `w-100` / `w-auto` | `w-full` / removed |
| `h-100` | removed |
| `mt-2/3/4`, `mb-2/3/4`, `mt-3`, `mb-4` | kept (CSS defines them) |
| `me-1/2/3`, `ms-2`, `p-3`, `py-3` | removed |
| `text-center` | `center-text` |
| `text-end` | `right-text` |
| `text-muted` | `muted` |
| `small` | `small` (kept) |
| `text-truncate` | `truncate` |
| `text-danger` / `text-success` | `text-danger` / `text-success` (kept) |
| `float-end` | wrap parent in `flex between` instead |
| `nowrap` | `nowrap` (kept) |
| `list-group` | `list` |
| `list-group-item` | `list-item` |
| `nav nav-tabs` / `nav-link` / `tab-content` / `tab-pane fade` | Alpine tabs (snippet R3) |
| `dropdown*`, `navbar*`, `collapse`, `spinner-border` | removed / replaced (Alpine, see per-task) |
| inline `style="background-color:..."` etc. | removed |

Inline `style="width:..."` / `style="max-width:..."` on table cells/headers may be kept where it carries column sizing.

### R3 — Alpine tabs snippet

For pages with tab strips (`feedback.html`, `settings.html`), use this pattern. `TABS_WRAPPER` is the element holding both the tab strip and the panes:

```html
<div x-data="{ tab: 'FIRST_TAB_ID' }">
    <div class="tabs">
        <button class="tab" :class="{ 'is-active': tab === 'FIRST_TAB_ID' }" @click="tab='FIRST_TAB_ID'">First</button>
        <button class="tab" :class="{ 'is-active': tab === 'SECOND_TAB_ID' }" @click="tab='SECOND_TAB_ID'">Second</button>
    </div>
    <div x-show="tab === 'FIRST_TAB_ID'">...pane 1...</div>
    <div x-show="tab === 'SECOND_TAB_ID'" x-cloak>...pane 2...</div>
</div>
```

The non-default panes get `x-cloak` so they are hidden before Alpine initializes.

### R4 — Pinned CDN versions (used in `heads.html`)

- HTMX: `https://unpkg.com/htmx.org@2.0.4`
- Alpine.js: `https://cdn.jsdelivr.net/npm/alpinejs@3.14.9/dist/cdn.min.js`
- bootstrap-icons: `https://cdn.jsdelivr.net/npm/bootstrap-icons@1.11.3/font/bootstrap-icons.css` (unchanged)
- Google Fonts: `https://fonts.googleapis.com/css2?family=Pixelify+Sans:wght@400;500;600;700&family=Press+Start+2P&display=swap`

---

### Task 1: CSS design system

**Files:**
- Modify (full rewrite): `app/webapi/assets/styles.css`

- [ ] **Step 1: Confirm the baseline test suite is green**

Run: `go test ./app/webapi/`
Expected: PASS (this is the parse gate; it must be green before and after the task).

- [ ] **Step 2: Replace `app/webapi/assets/styles.css` entirely with the design system**

```css
/* ============================================================
   Shield web UI design system.
   dark ops-console theme with a tasteful Minecraft flavor.
   organization: tokens, reset/base, layout, components, utilities.
   ============================================================ */

/* tokens */
:root {
    --bg: #0e1116;
    --surface: #161b22;
    --elevated: #1c2230;
    --border: #2a313c;
    --border-strong: #3a4350;

    --text: #e6e9ee;
    --text-muted: #9aa4b2;

    --accent: #4f7cff;
    --accent-hover: #6b91ff;
    --accent-press: #3c63d6;

    --spam: #d83a3a;
    --ham: #4caf50;
    --gold: #e0a526;
    --info: #3aa0d8;

    --space-1: 4px;
    --space-2: 8px;
    --space-3: 12px;
    --space-4: 16px;
    --space-6: 24px;
    --space-8: 32px;

    --radius: 2px;
    --sidebar-w: 240px;

    --font-body: system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
    --font-head: "Pixelify Sans", system-ui, sans-serif;
    --font-brand: "Press Start 2P", ui-monospace, monospace;
    --font-mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;

    --bevel-light: rgba(255, 255, 255, 0.13);
    --bevel-dark: rgba(0, 0, 0, 0.5);
}

[x-cloak] { display: none !important; }

/* reset / base */
*, *::before, *::after { box-sizing: border-box; }

html, body { height: 100%; }

body {
    margin: 0;
    background-color: var(--bg);
    color: var(--text);
    font-family: var(--font-body);
    font-size: 15px;
    line-height: 1.5;
    -webkit-font-smoothing: antialiased;
}

h1, h2, h3, h4, h5 {
    font-family: var(--font-head);
    font-weight: 600;
    letter-spacing: 0.5px;
    margin: 0 0 var(--space-3);
}

a { color: var(--accent); text-decoration: none; }
a:hover { color: var(--accent-hover); }

code, pre, .mono { font-family: var(--font-mono); }

code {
    background-color: var(--elevated);
    padding: 1px 5px;
    border-radius: var(--radius);
    font-size: 0.9em;
}

pre { margin: 0; }

img { image-rendering: pixelated; }

/* layout: sidebar */
.sidebar {
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    width: var(--sidebar-w);
    background-color: var(--surface);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    z-index: 30;
}

.sidebar__brand {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-4);
    border-bottom: 1px solid var(--border);
}

.sidebar__brand img { width: 28px; height: 28px; }

.sidebar__brand-name {
    font-family: var(--font-brand);
    font-size: 13px;
    color: var(--accent);
}

.sidebar__nav {
    list-style: none;
    margin: 0;
    padding: var(--space-2) 0;
    flex: 1;
    overflow-y: auto;
}

.sidebar__footer {
    border-top: 1px solid var(--border);
    padding: var(--space-2) 0;
}

.nav-link {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-4);
    color: var(--text-muted);
    border-left: 3px solid transparent;
    cursor: pointer;
}

.nav-link:hover {
    color: var(--text);
    background-color: var(--elevated);
}

.nav-link.is-active {
    color: var(--text);
    background-color: var(--elevated);
    border-left-color: var(--accent);
}

.nav-link i { font-size: 1.05rem; }

/* layout: main + topbar */
.main {
    margin-left: var(--sidebar-w);
    min-height: 100vh;
    display: flex;
    flex-direction: column;
}

.topbar {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-6);
    background-color: var(--surface);
    border-bottom: 1px solid var(--border);
}

.topbar__burger {
    display: none;
    background: none;
    border: none;
    color: var(--text);
    font-size: 1.4rem;
    cursor: pointer;
}

.page-title {
    font-size: 1.25rem;
    margin: 0;
}

.content {
    padding: var(--space-6);
    max-width: 1100px;
    width: 100%;
}

.scrim {
    position: fixed;
    inset: 0;
    background-color: rgba(0, 0, 0, 0.55);
    z-index: 20;
}

/* buttons */
.btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-4);
    font-family: var(--font-body);
    font-size: 0.92rem;
    font-weight: 600;
    color: var(--text);
    background-color: var(--elevated);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    cursor: pointer;
    box-shadow: inset 1px 1px 0 var(--bevel-light), inset -2px -2px 0 var(--bevel-dark);
    transition: filter 0.1s ease;
}

.btn:hover { filter: brightness(1.15); }

.btn:active {
    box-shadow: inset -1px -1px 0 var(--bevel-light), inset 2px 2px 0 var(--bevel-dark);
}

.btn:disabled { opacity: 0.5; cursor: not-allowed; }

.btn-accent { background-color: var(--accent); border-color: var(--accent-press); color: #fff; }
.btn-danger { background-color: var(--spam); border-color: #a52828; color: #fff; }
.btn-ham { background-color: var(--ham); border-color: #3a8c3e; color: #fff; }
.btn-warn { background-color: var(--gold); border-color: #b9871c; color: #1a1a1a; }
.btn-ghost { background-color: transparent; box-shadow: none; }
.btn-ghost:hover { background-color: var(--elevated); }
.btn-sm { padding: var(--space-1) var(--space-2); font-size: 0.82rem; }

/* cards */
.card {
    background-color: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    margin-bottom: var(--space-4);
}

.card-header {
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border);
    font-family: var(--font-head);
    font-weight: 600;
    display: flex;
    align-items: center;
    justify-content: space-between;
}

.card-body { padding: var(--space-4); }

/* tables */
.table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.9rem;
}

.table th, .table td {
    padding: var(--space-2) var(--space-3);
    text-align: left;
    border-bottom: 1px solid var(--border);
    vertical-align: middle;
    word-break: break-word;
}

.table thead th {
    background-color: var(--elevated);
    color: var(--text);
    font-family: var(--font-head);
    white-space: nowrap;
}

.table tbody tr:nth-child(even) { background-color: rgba(255, 255, 255, 0.02); }
.table tbody tr:hover { background-color: var(--elevated); }

.table-wrap { overflow-x: auto; }

/* forms */
.field { margin-bottom: var(--space-3); }

.field label, .form-label {
    display: block;
    margin-bottom: var(--space-1);
    color: var(--text-muted);
    font-size: 0.85rem;
}

.input, .select {
    width: 100%;
    padding: var(--space-2) var(--space-3);
    background-color: var(--bg);
    color: var(--text);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    font-family: var(--font-body);
    font-size: 0.92rem;
}

.input:focus, .select:focus { outline: none; border-color: var(--accent); }

textarea.input { resize: vertical; min-height: 70px; }

.checkbox { width: 16px; height: 16px; accent-color: var(--accent); }

.form-row {
    display: flex;
    gap: var(--space-4);
    align-items: flex-start;
    margin-bottom: var(--space-3);
}

.form-row > label { flex: 0 0 220px; padding-top: 6px; color: var(--text-muted); }
.form-row__control { flex: 1; }

/* alerts */
.alert {
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius);
    border-left: 4px solid var(--border-strong);
    background-color: var(--elevated);
    margin-bottom: var(--space-3);
}

.alert-danger { border-left-color: var(--spam); }
.alert-success { border-left-color: var(--ham); }
.alert-warn { border-left-color: var(--gold); }
.alert-info { border-left-color: var(--info); }

/* badges */
.badge {
    display: inline-block;
    padding: 2px 7px;
    font-size: 0.75rem;
    font-weight: 600;
    border-radius: var(--radius);
    background-color: var(--border-strong);
    color: var(--text);
}

.badge-spam { background-color: var(--spam); color: #fff; }
.badge-ham { background-color: var(--ham); color: #fff; }
.badge-warn { background-color: var(--gold); color: #1a1a1a; }
.badge-info { background-color: var(--info); color: #fff; }
.badge-muted { background-color: var(--border-strong); color: var(--text-muted); }

/* tabs */
.tabs {
    display: flex;
    gap: var(--space-1);
    border-bottom: 1px solid var(--border);
    margin-bottom: var(--space-4);
    flex-wrap: wrap;
}

.tab {
    padding: var(--space-2) var(--space-4);
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    color: var(--text-muted);
    font-family: var(--font-head);
    font-size: 0.92rem;
    cursor: pointer;
}

.tab:hover { color: var(--text); }
.tab.is-active { color: var(--text); border-bottom-color: var(--accent); }

/* list */
.list { list-style: none; margin: 0; padding: 0; }

.list-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    background-color: var(--surface);
    border: 1px solid var(--border);
    border-bottom: none;
}

.list-item:last-child { border-bottom: 1px solid var(--border); }

/* detected-spam table column sizing */
.ds-timestamp { min-width: 50px; max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ds-text { min-width: 200px; max-width: 500px; width: 500px; word-break: break-word; hyphens: auto; }
.ds-username { min-width: 50px; max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ds-checks { font-size: 0.85rem; width: 300px; max-width: 300px; word-break: break-word; }
.ds-checks > div { margin-bottom: 2px; }

@media (max-width: 1200px) {
    .ds-text { max-width: 350px; width: 350px; }
    .ds-checks { max-width: 240px; width: 240px; }
}

/* htmx indicator */
.htmx-indicator { opacity: 0; transition: opacity 400ms ease-in; }
.htmx-request .htmx-indicator,
.htmx-request.htmx-indicator { opacity: 1; }

/* utilities */
.flex { display: flex; }
.flex-col { display: flex; flex-direction: column; }
.between { justify-content: space-between; }
.center { align-items: center; }
.end { justify-content: flex-end; }
.wrap { flex-wrap: wrap; }
.grow { flex: 1; }
.gap-1 { gap: var(--space-1); }
.gap-2 { gap: var(--space-2); }
.gap-3 { gap: var(--space-3); }
.gap-4 { gap: var(--space-4); }

.grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-4); }
.grid-2-1 { display: grid; grid-template-columns: 2fr 1fr; gap: var(--space-4); }

.filter-bar { display: flex; flex-wrap: wrap; gap: var(--space-3); align-items: flex-end; margin-bottom: var(--space-4); }

.mt-2 { margin-top: var(--space-2); }
.mt-3 { margin-top: var(--space-3); }
.mt-4 { margin-top: var(--space-4); }
.mb-2 { margin-bottom: var(--space-2); }
.mb-3 { margin-bottom: var(--space-3); }
.mb-4 { margin-bottom: var(--space-4); }
.mb-6 { margin-bottom: var(--space-6); }

.muted { color: var(--text-muted); }
.small { font-size: 0.82rem; }
.center-text { text-align: center; }
.right-text { text-align: right; }
.nowrap { white-space: nowrap; }
.truncate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 300px; }
.w-full { width: 100%; }
.hidden { display: none !important; }

.text-danger { color: var(--spam); }
.text-success { color: var(--ham); }

.only-mobile { display: none; }

/* responsive */
@media (max-width: 860px) {
    .sidebar { transform: translateX(-100%); transition: transform 0.18s ease; }
    .sidebar.is-open { transform: translateX(0); }
    .main { margin-left: 0; }
    .topbar__burger { display: block; }
    .content { padding: var(--space-4); }
    .grid-2, .grid-2-1 { grid-template-columns: 1fr; }
    .only-mobile { display: block; }
    .only-desktop { display: none; }
}
```

- [ ] **Step 3: Verify the test suite still passes**

Run: `go test ./app/webapi/`
Expected: PASS (CSS is not compiled into Go; this confirms nothing else broke).

- [ ] **Step 4: Commit**

```bash
git add app/webapi/assets/styles.css
git commit -m "feat(webui): add ops-console CSS design system"
```

---

### Task 2: Head dependencies, app.js, static route

**Files:**
- Modify (full rewrite): `app/webapi/assets/components/heads.html`
- Create: `app/webapi/assets/app.js`
- Modify: `app/webapi/routes.go:149-153`

- [ ] **Step 1: Replace `app/webapi/assets/components/heads.html`**

```html
<link href="https://cdn.jsdelivr.net/npm/bootstrap-icons@1.11.3/font/bootstrap-icons.css" rel="stylesheet">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Pixelify+Sans:wght@400;500;600;700&family=Press+Start+2P&display=swap" rel="stylesheet">
<link href="styles.css" rel="stylesheet">
<script defer src="https://unpkg.com/htmx.org@2.0.4"></script>
<script defer src="https://cdn.jsdelivr.net/npm/alpinejs@3.14.9/dist/cdn.min.js"></script>
<script defer src="app.js"></script>
```

- [ ] **Step 2: Create `app/webapi/assets/app.js`**

```javascript
// global helpers for the Shield web ui.

// copyUserID copies a telegram user id to the clipboard and gives the button feedback.
function copyUserID(userId, btn) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(String(userId)).then(function () {
            if (btn) {
                btn.textContent = 'Copied!';
                btn.classList.add('btn-ham');
                setTimeout(function () {
                    btn.textContent = 'Copy ID';
                    btn.classList.remove('btn-ham');
                }, 2000);
            }
        }).catch(function () {
            if (btn) {
                btn.textContent = 'Failed to copy';
                setTimeout(function () { btn.textContent = 'Copy ID'; }, 2000);
            }
        });
    } else if (btn) {
        btn.textContent = 'Failed to copy';
        setTimeout(function () { btn.textContent = 'Copy ID'; }, 2000);
    }
}

// re-initialize Alpine components inside fragments swapped in by HTMX.
document.body.addEventListener('htmx:afterSwap', function (evt) {
    if (window.Alpine && evt.detail && evt.detail.target) {
        window.Alpine.initTree(evt.detail.target);
    }
});
```

- [ ] **Step 3: Add the `app.js` static-file mapping in `app/webapi/routes.go`**

Replace the `newStaticFS` block at `app/webapi/routes.go:149-153`:

```go
		staticFiles := newStaticFS(templateFS,
			staticFileMapping{urlPath: "styles.css", filesysPath: "assets/styles.css"},
			staticFileMapping{urlPath: "app.js", filesysPath: "assets/app.js"},
			staticFileMapping{urlPath: "logo.png", filesysPath: "assets/logo.png"},
			staticFileMapping{urlPath: "spinner.svg", filesysPath: "assets/spinner.svg"},
		)
```

(`app.js` lives in `assets/`, which is already covered by the `//go:embed assets/* assets/components/*` directive at `app/webapi/webapi.go:40`; non-`.html` files are not parsed as templates.)

- [ ] **Step 4: Verify build and test suite**

Run: `go build ./... && go test ./app/webapi/`
Expected: PASS. (Pages still reference Bootstrap classes and now render unstyled — that is expected; the branch merges whole.)

- [ ] **Step 5: Commit**

```bash
git add app/webapi/assets/components/heads.html app/webapi/assets/app.js app/webapi/routes.go
git commit -m "feat(webui): drop Bootstrap deps, add Alpine and app.js"
```

---

### Task 3: Sidebar component

**Files:**
- Create: `app/webapi/assets/components/sidebar.html`

- [ ] **Step 1: Create `app/webapi/assets/components/sidebar.html`**

```html
<aside class="sidebar" :class="{ 'is-open': navOpen }">
    <div class="sidebar__brand">
        <img src="/logo.png" alt="Shield logo">
        <span class="sidebar__brand-name">Shield</span>
    </div>
    <ul class="sidebar__nav">
        <li><a class="nav-link" :class="{ 'is-active': window.location.pathname === '/' }" href="/"><i class="bi bi-shield-check"></i>Checker</a></li>
        <li><a class="nav-link" :class="{ 'is-active': window.location.pathname === '/manage_samples' }" href="/manage_samples"><i class="bi bi-collection"></i>Manage Samples</a></li>
        <li><a class="nav-link" :class="{ 'is-active': window.location.pathname === '/manage_users' }" href="/manage_users"><i class="bi bi-people"></i>Manage Users</a></li>
        <li><a class="nav-link" :class="{ 'is-active': window.location.pathname === '/manage_dictionary' }" href="/manage_dictionary"><i class="bi bi-book"></i>Manage Dictionary</a></li>
        <li><a class="nav-link" :class="{ 'is-active': window.location.pathname === '/detected_spam' }" href="/detected_spam"><i class="bi bi-exclamation-triangle"></i>Detected Spam</a></li>
        <li><a class="nav-link" :class="{ 'is-active': window.location.pathname === '/incidents' }" href="/incidents"><i class="bi bi-clipboard-data"></i>Incidents</a></li>
        <li><a class="nav-link" :class="{ 'is-active': window.location.pathname === '/appeals' }" href="/appeals"><i class="bi bi-chat-left-text"></i>Appeals</a></li>
        <li><a class="nav-link" :class="{ 'is-active': window.location.pathname === '/feedback' }" href="/feedback"><i class="bi bi-lightbulb"></i>Feedback</a></li>
        <li><a class="nav-link" :class="{ 'is-active': window.location.pathname === '/list_settings' }" href="/list_settings"><i class="bi bi-gear"></i>Settings</a></li>
    </ul>
    <div class="sidebar__footer">
        <a class="nav-link" href="/download/backup" download><i class="bi bi-download"></i>Create Backup</a>
        <a class="nav-link" hx-get="/logout" hx-push-url="true"><i class="bi bi-box-arrow-right"></i>Logout</a>
    </div>
</aside>
```

`navOpen` resolves against the `x-data` on each page's `<body>` (R1). `window.location.pathname` is read directly in the Alpine `:class` expression — no `x-data` is needed for it. `navbar.html` is left in place until Task 15 (it is no longer referenced once all pages are converted, but keeping it avoids a parse error if any page lags).

- [ ] **Step 2: Verify the test suite still passes**

Run: `go test ./app/webapi/`
Expected: PASS (new component parses; no page references it yet).

- [ ] **Step 3: Commit**

```bash
git add app/webapi/assets/components/sidebar.html
git commit -m "feat(webui): add left sidebar component"
```

---

### Task 4: Convert `spam_check.html`

**Files:**
- Modify (full rewrite): `app/webapi/assets/spam_check.html`

- [ ] **Step 1: Replace `app/webapi/assets/spam_check.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>Checker - Shield</title>
    {{template "heads.html"}}
</head>
<body x-data="{ navOpen: false }">
{{template "sidebar.html"}}
<div class="scrim" x-show="navOpen" @click="navOpen=false" x-cloak></div>
<main class="main">
    <header class="topbar">
        <button class="topbar__burger" @click="navOpen=!navOpen" aria-label="Toggle navigation"><i class="bi bi-list"></i></button>
        <h2 class="page-title">Message Checker</h2>
    </header>
    <div class="content">
        <p class="muted mb-4">Version: {{.Version}}</p>
        <div class="card" style="max-width:560px">
            <div class="card-body">
                <form hx-post="/check" hx-target="#result" hx-encoding="json" hx-error="#error-message" hx-swap="outerHTML">
                    <div class="field">
                        <label for="userId">User ID</label>
                        <input type="text" class="input" id="userId" name="user_id" placeholder="Enter User ID">
                    </div>
                    <div class="field">
                        <label for="message">Message</label>
                        <textarea id="message" name="msg" class="input" placeholder="Enter message to check" rows="4"></textarea>
                    </div>
                    <button type="submit" class="btn btn-accent w-full">Check</button>
                </form>
                <div id="result">
                    <div id="error-message" class="alert alert-danger hidden" role="alert"></div>
                </div>
            </div>
        </div>
    </div>
</main>
</body>
</html>

{{define "check_results"}}
<div id="result">
    <div id="error-message"></div>
    <div class="alert {{if .Spam}}alert-danger{{else}}alert-success{{end}}">
        <strong>Result:</strong> {{if .Spam}}Spam detected{{else}}No spam detected{{end}}
    </div>
    {{range .Checks}}
    <div class="mb-2 {{if .Spam}}text-danger{{else}}text-success{{end}}">
        <strong>{{.Name}}:</strong> {{.Details}}
    </div>
    {{end}}
</div>
{{end}}
```

- [ ] **Step 2: Verify**

Run: `go test ./app/webapi/ -run TestServer_htmlSpamCheckHandler`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add app/webapi/assets/spam_check.html
git commit -m "feat(webui): restyle checker page"
```

---

### Task 5: Convert `manage_samples.html`

**Files:**
- Modify (full rewrite): `app/webapi/assets/manage_samples.html`

- [ ] **Step 1: Replace `app/webapi/assets/manage_samples.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>Manage Samples - Shield</title>
    {{template "heads.html"}}
</head>
<body x-data="{ navOpen: false }">
{{template "sidebar.html"}}
<div class="scrim" x-show="navOpen" @click="navOpen=false" x-cloak></div>
<main class="main">
    <header class="topbar">
        <button class="topbar__burger" @click="navOpen=!navOpen" aria-label="Toggle navigation"><i class="bi bi-list"></i></button>
        <h2 class="page-title">Manage Samples</h2>
    </header>
    <div class="content">
        <div class="grid-2 mb-4">
            <form hx-post="/update/spam" hx-target="#samples-list" hx-swap="innerHTML" hx-on::after-request="this.reset()">
                <textarea name="msg" class="input mb-2" placeholder="Enter spam sample"></textarea>
                <button type="submit" class="btn btn-danger">Add Spam</button>
            </form>
            <form hx-post="/update/ham" hx-target="#samples-list" hx-swap="innerHTML" hx-on::after-request="this.reset()">
                <textarea name="msg" class="input mb-2" placeholder="Enter ham sample"></textarea>
                <button type="submit" class="btn btn-ham">Add Ham</button>
            </form>
        </div>

        {{template "samples_list" .}}
    </div>
</main>
</body>
</html>

{{define "samples_list"}}
<div class="grid-2" id="samples-list">
    <div>
        <h4 class="flex center gap-2">
            Spam Samples ({{.TotalSpamSamples}})
            <a href="/download/spam" class="btn btn-ghost btn-sm">Download</a>
        </h4>
        <ul class="list" id="spam-samples-list">
            {{range .SpamSamples}}
            <li class="list-item">
                <span id="loading-{{.ID}}">{{.Sample}} <img class="htmx-indicator" src="/spinner.svg"/></span>
                <form method="POST" hx-post="/delete/spam" hx-target="#samples-list" hx-swap="outerHTML" hx-indicator="#loading-{{.ID}}">
                    <input type="hidden" name="msg" value="{{.Sample}}">
                    <button type="submit" class="btn btn-sm btn-danger"><i class="bi bi-trash"></i></button>
                </form>
            </li>
            {{else}}
            <li class="list-item">No spam samples found</li>
            {{end}}
        </ul>
    </div>
    <div>
        <h4 class="flex center gap-2">
            Ham Samples ({{.TotalHamSamples}})
            <a href="/download/ham" class="btn btn-ghost btn-sm">Download</a>
        </h4>
        <ul class="list" id="ham-samples-list">
            {{range .HamSamples}}
            <li class="list-item">
                {{.Sample}}
                <form method="POST" hx-post="/delete/ham" hx-target="#samples-list" hx-swap="outerHTML">
                    <input type="hidden" name="msg" value="{{.Sample}}">
                    <button type="submit" class="btn btn-sm btn-danger"><i class="bi bi-trash"></i></button>
                </form>
            </li>
            {{else}}
            <li class="list-item">No ham samples found</li>
            {{end}}
        </ul>
    </div>
</div>
{{end}}
```

- [ ] **Step 2: Verify**

Run: `go test ./app/webapi/ -run TestServer_htmlManageSamplesHandler`
Expected: FAIL — `webapi_part4_test.go:141` still asserts `<div class="row" id="samples-list">`. This assertion is fixed in Task 16. Confirm the failure is **only** that string assertion, not a template parse panic.

- [ ] **Step 3: Commit**

```bash
git add app/webapi/assets/manage_samples.html
git commit -m "feat(webui): restyle manage samples page"
```

---

### Task 6: Convert `manage_users.html`

**Files:**
- Modify (full rewrite): `app/webapi/assets/manage_users.html`

- [ ] **Step 1: Replace `app/webapi/assets/manage_users.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>Manage Users - Shield</title>
    {{template "heads.html"}}
</head>
<body x-data="{ navOpen: false }">
{{template "sidebar.html"}}
<div class="scrim" x-show="navOpen" @click="navOpen=false" x-cloak></div>
<main class="main">
    <header class="topbar">
        <button class="topbar__burger" @click="navOpen=!navOpen" aria-label="Toggle navigation"><i class="bi bi-list"></i></button>
        <h2 class="page-title">Manage Approved Users</h2>
    </header>
    <div class="content">
        <form class="flex wrap gap-2 center mb-4" hx-post="/users/add" hx-target="#users-list" hx-swap="outerHTML" hx-error="#error-message" hx-on::after-request="this.reset()">
            <input type="text" name="user_id" class="input" style="width:auto" placeholder="User ID">
            <input type="text" name="user_name" class="input" style="width:auto" placeholder="User Name">
            <button type="submit" class="btn btn-accent"><i class="bi bi-plus-circle"></i> Add to Approved Users</button>
        </form>

        {{template "users_list" .}}
    </div>
</main>
</body>
</html>

{{define "users_list"}}
<div id="users-list">
    <div id="error-message"></div>
    <h4>Approved Users ({{.TotalApprovedUsers}})</h4>
    <div class="table-wrap">
        <table class="table">
            <thead>
            <tr>
                <th>ID</th>
                <th>Name</th>
                <th>Timestamp</th>
                <th></th>
            </tr>
            </thead>
            <tbody>
            {{range .ApprovedUsers}}
            <tr>
                <td>{{.UserID}}</td>
                <td>{{.UserName}}</td>
                <td>{{.Timestamp.Format "2006-01-02 15:04:05"}}</td>
                <td class="right-text">
                    <form method="POST" hx-post="/users/delete" hx-target="#users-list" hx-swap="outerHTML">
                        <input type="hidden" name="user_id" value="{{.UserID}}">
                        <button type="submit" class="btn btn-danger btn-sm" title="Delete User"><i class="bi bi-trash"></i></button>
                    </form>
                </td>
            </tr>
            {{else}}
            <tr>
                <td colspan="4">No approved users found</td>
            </tr>
            {{end}}
            </tbody>
        </table>
    </div>
</div>
{{end}}
```

- [ ] **Step 2: Verify**

Run: `go test ./app/webapi/ -run TestServer_htmlManageUsersHandler`
Expected: PASS (the test asserts `<title>Manage Users - TG-Spam</title>` at `webapi_part4_test.go:165` — this fails until Task 16). Confirm the only failure is that title string, not a parse panic. If `go test` reports a parse panic, fix the template; otherwise proceed.

- [ ] **Step 3: Commit**

```bash
git add app/webapi/assets/manage_users.html
git commit -m "feat(webui): restyle manage users page"
```

---

### Task 7: Convert `manage_dictionary.html`

**Files:**
- Modify (full rewrite): `app/webapi/assets/manage_dictionary.html`

- [ ] **Step 1: Replace `app/webapi/assets/manage_dictionary.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>Manage Dictionary - Shield</title>
    {{template "heads.html"}}
</head>
<body x-data="{ navOpen: false }">
{{template "sidebar.html"}}
<div class="scrim" x-show="navOpen" @click="navOpen=false" x-cloak></div>
<main class="main">
    <header class="topbar">
        <button class="topbar__burger" @click="navOpen=!navOpen" aria-label="Toggle navigation"><i class="bi bi-list"></i></button>
        <h2 class="page-title">Manage Dictionary</h2>
    </header>
    <div class="content">
        <div id="error-message"></div>

        <div class="grid-2 mb-4">
            <form hx-post="/dictionary/add" hx-target="#dictionary-list" hx-swap="innerHTML" hx-on::after-request="this.reset()">
                <input type="hidden" name="type" value="stop_phrase">
                <textarea name="data" class="input mb-2" placeholder="Enter stop phrase"></textarea>
                <button type="submit" class="btn btn-danger">Add Stop Phrase</button>
            </form>
            <form hx-post="/dictionary/add" hx-target="#dictionary-list" hx-swap="innerHTML" hx-on::after-request="this.reset()">
                <input type="hidden" name="type" value="ignored_word">
                <textarea name="data" class="input mb-2" placeholder="Enter ignored word"></textarea>
                <button type="submit" class="btn btn-ham">Add Ignored Word</button>
            </form>
        </div>

        {{template "dictionary_list" .}}
    </div>
</main>
</body>
</html>

{{define "dictionary_list"}}
<div class="grid-2" id="dictionary-list">
    <div>
        <h4>Stop Phrases ({{.TotalStopPhrases}})</h4>
        <ul class="list" id="stop-phrases-list">
            {{range .StopPhrases}}
            <li class="list-item">
                <span id="loading-stop-{{.ID}}">{{.Data}} <img class="htmx-indicator" src="/spinner.svg"/></span>
                <form method="POST" hx-post="/dictionary/delete" hx-target="#dictionary-list" hx-swap="outerHTML" hx-indicator="#loading-stop-{{.ID}}">
                    <input type="hidden" name="id" value="{{.ID}}">
                    <button type="submit" class="btn btn-sm btn-danger"><i class="bi bi-trash"></i></button>
                </form>
            </li>
            {{else}}
            <li class="list-item">No stop phrases found</li>
            {{end}}
        </ul>
    </div>
    <div>
        <h4>Ignored Words ({{.TotalIgnoredWords}})</h4>
        <ul class="list" id="ignored-words-list">
            {{range .IgnoredWords}}
            <li class="list-item">
                <span id="loading-ignored-{{.ID}}">{{.Data}} <img class="htmx-indicator" src="/spinner.svg"/></span>
                <form method="POST" hx-post="/dictionary/delete" hx-target="#dictionary-list" hx-swap="outerHTML" hx-indicator="#loading-ignored-{{.ID}}">
                    <input type="hidden" name="id" value="{{.ID}}">
                    <button type="submit" class="btn btn-sm btn-danger"><i class="bi bi-trash"></i></button>
                </form>
            </li>
            {{else}}
            <li class="list-item">No ignored words found</li>
            {{end}}
        </ul>
    </div>
</div>
{{end}}
```

- [ ] **Step 2: Verify**

Run: `go test ./app/webapi/`
Expected: PASS for dictionary-related tests; the only failures in the package are the title/markup assertions noted in Task 16. Confirm no parse panic.

- [ ] **Step 3: Commit**

```bash
git add app/webapi/assets/manage_dictionary.html
git commit -m "feat(webui): restyle manage dictionary page"
```

---

### Task 8: Convert `detected_spam.html`

**Files:**
- Modify (full rewrite): `app/webapi/assets/detected_spam.html`

- [ ] **Step 1: Replace `app/webapi/assets/detected_spam.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>Detected Spam - Shield</title>
    {{template "heads.html"}}
</head>
<body x-data="{ navOpen: false }">
{{template "sidebar.html"}}
<div class="scrim" x-show="navOpen" @click="navOpen=false" x-cloak></div>
<main class="main">
    <header class="topbar">
        <button class="topbar__burger" @click="navOpen=!navOpen" aria-label="Toggle navigation"><i class="bi bi-list"></i></button>
        <h2 class="page-title">Detected Spam</h2>
    </header>
    <div class="content">
        <div id="error-message" class="alert alert-danger hidden" role="alert"></div>
        <div class="flex between center wrap gap-3 mb-3">
            <h4 class="flex center gap-2">
                <span class="nowrap">Detected Spam <span id="count-display">{{if ne .Filter "all"}}({{.FilteredCount}}/{{.TotalDetectedSpam}}){{else}}({{.TotalDetectedSpam}}){{end}}</span></span>
                <a href="/download/detected_spam" class="btn btn-ghost btn-sm">Download</a>
            </h4>
            <div class="flex center gap-2">
                <label for="filter-select">Filter:</label>
                <select id="filter-select" name="filter" class="select" style="width:auto"
                        hx-get="/detected_spam" hx-trigger="change" hx-target="#spam-list-content">
                    <option value="all" {{if eq .Filter "all"}}selected{{end}}>All</option>
                    <option value="non-classified" {{if eq .Filter "non-classified"}}selected{{end}}>Missed by Classifier</option>
                    {{if .OpenAIEnabled}}
                    <option value="openai" {{if eq .Filter "openai"}}selected{{end}}>OpenAI</option>
                    {{end}}
                    {{if .GeminiEnabled}}
                    <option value="gemini" {{if eq .Filter "gemini"}}selected{{end}}>Gemini</option>
                    {{end}}
                </select>
            </div>
        </div>

        <div id="spam-list">
            <div id="spam-list-content">
                {{template "detected_spam_content" .}}
            </div>
            <div id="spam-count" class="hidden">
                {{if ne .Filter "all"}}({{.FilteredCount}}/{{.TotalDetectedSpam}}){{else}}({{.TotalDetectedSpam}}){{end}}
            </div>
        </div>
    </div>
</main>
</body>
</html>

{{define "detected_spam_content"}}
<div class="only-desktop">
    <div class="table-wrap">
        <table class="table">
            <thead>
            <tr>
                <th>Timestamp</th>
                <th>User ID</th>
                <th>User Name</th>
                <th>Text</th>
                <th>Checks</th>
            </tr>
            </thead>
            <tbody>
            {{range .DetectedSpamEntries}}
            {{$text := .Text}}
            {{$id := .ID}}
            {{$added := .Added}}
            <tr>
                <td class="ds-timestamp">{{.Timestamp.Format "2006-01-02 15:04:05"}}</td>
                <td>{{.UserID}}</td>
                <td class="ds-username">{{.UserName}}</td>
                <td class="ds-text">{{.Text}}</td>
                <td class="ds-checks">
                    {{$hasClassifier := false}}
                    {{range .Checks}}
                        {{if and (not .Spam) (not $added) (eq .Name "classifier")}}
                            {{$hasClassifier = true}}
                        {{end}}
                    {{end}}
                    {{range .Checks}}
                    <div class="{{if .Spam}}text-danger{{else}}text-success{{end}}">
                        <strong>{{.Name}}:</strong> {{.Details}}
                    </div>
                    {{end}}
                    {{if and (not $added) $hasClassifier}}
                    <div class="mt-2">
                        <button hx-post="/detected_spam/add" hx-vals='{"id": {{$id}}, "msg": "{{$text}}"}'
                                hx-target="this" hx-error="#error-message" hx-swap="outerHTML"
                                class="btn btn-sm btn-warn" title="Add this message to spam samples">
                            Add to spam samples
                        </button>
                    </div>
                    {{end}}
                </td>
            </tr>
            {{else}}
            <tr>
                <td colspan="5">No detected spam found</td>
            </tr>
            {{end}}
            </tbody>
        </table>
    </div>
</div>

<div class="only-mobile">
    {{range .DetectedSpamEntries}}
    {{$text := .Text}}
    {{$id := .ID}}
    {{$added := .Added}}
    <div class="card">
        <div class="card-header">
            <span><strong>{{.UserName}}</strong> ({{.UserID}})</span>
            <span class="small muted">{{.Timestamp.Format "2006-01-02 15:04"}}</span>
        </div>
        <div class="card-body">
            <p>{{.Text}}</p>
            <div class="small">
                {{$hasClassifier := false}}
                {{range .Checks}}
                    {{if and (not .Spam) (not $added) (eq .Name "classifier")}}
                        {{$hasClassifier = true}}
                    {{end}}
                {{end}}
                {{range .Checks}}
                <div class="{{if .Spam}}text-danger{{else}}text-success{{end}}">
                    <strong>{{.Name}}:</strong> {{.Details}}
                </div>
                {{end}}
                {{if and (not $added) $hasClassifier}}
                <div class="mt-2">
                    <button hx-post="/detected_spam/add" hx-vals='{"id": {{$id}}, "msg": "{{$text}}"}'
                            hx-target="this" hx-error="#error-message" hx-swap="outerHTML"
                            class="btn btn-sm btn-warn" title="Add this message to spam samples">
                        Add to spam samples
                    </button>
                </div>
                {{end}}
            </div>
        </div>
    </div>
    {{else}}
    <div class="alert alert-info">No detected spam found</div>
    {{end}}
</div>
{{end}}
```

- [ ] **Step 2: Verify**

Run: `go test ./app/webapi/ -run TestServer_htmlDetectedSpamHandler`
Expected: FAIL — `webapi_part3_test.go:335` still asserts `btn-custom-blue`. Fixed in Task 16. Confirm the only failure is that assertion, not a parse panic.

- [ ] **Step 3: Commit**

```bash
git add app/webapi/assets/detected_spam.html
git commit -m "feat(webui): restyle detected spam page"
```

---

### Task 9: Convert `incidents.html`

**Files:**
- Modify (full rewrite): `app/webapi/assets/incidents.html`

- [ ] **Step 1: Replace `app/webapi/assets/incidents.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>Incidents - Shield</title>
    {{template "heads.html"}}
</head>
<body x-data="{ navOpen: false }">
{{template "sidebar.html"}}
<div class="scrim" x-show="navOpen" @click="navOpen=false" x-cloak></div>
<main class="main">
    <header class="topbar">
        <button class="topbar__burger" @click="navOpen=!navOpen" aria-label="Toggle navigation"><i class="bi bi-list"></i></button>
        <h2 class="page-title">Incidents</h2>
    </header>
    <div class="content">
        <form id="filter-form" class="filter-bar" hx-get="/incidents" hx-target="#incidents-content" hx-swap="innerHTML">
            <div class="field" style="margin-bottom:0">
                <label>Status</label>
                <select name="status" class="select">
                    <option value="">All</option>
                    <option value="open">Open</option>
                    <option value="reviewing">Reviewing</option>
                    <option value="appealed">Appealed</option>
                    <option value="resolved">Resolved</option>
                    <option value="closed">Closed</option>
                </select>
            </div>
            <div class="field" style="margin-bottom:0">
                <label>Source</label>
                <select name="source" class="select">
                    <option value="">All</option>
                    <option value="auto_mod">Auto Mod</option>
                    <option value="user_report">User Report</option>
                    <option value="admin_action">Admin Action</option>
                </select>
            </div>
            <div class="field" style="margin-bottom:0">
                <label>Limit</label>
                <select name="limit" class="select">
                    <option value="50">50</option>
                    <option value="100">100</option>
                    <option value="200">200</option>
                </select>
            </div>
            <button type="submit" class="btn btn-accent btn-sm">Filter</button>
        </form>

        <div id="incidents-content">
            {{if .Incidents}}
            <div class="table-wrap">
                <table class="table">
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>Time</th>
                            <th>User</th>
                            <th>Source</th>
                            <th>Status</th>
                            <th>Severity</th>
                            <th>Reason</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{range .Incidents}}
                        <tr>
                            <td><a href="/incidents/{{.ID}}">{{.ID}}</a></td>
                            <td>{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
                            <td>{{.SpamUserName}} <span class="small muted">({{.SpamUserID}})</span></td>
                            <td><span class="badge badge-muted">{{.Source}}</span></td>
                            <td>
                                <span class="badge {{if eq .Status "open"}}badge-warn{{else if eq .Status "reviewing"}}badge-info{{else if eq .Status "appealed"}}badge-info{{else if eq .Status "resolved"}}badge-ham{{else}}badge-muted{{end}}">
                                    {{.Status}}
                                </span>
                            </td>
                            <td>
                                <span class="badge {{if eq .Severity "critical"}}badge-spam{{else if eq .Severity "high"}}badge-warn{{else if eq .Severity "medium"}}badge-info{{else}}badge-muted{{end}}">
                                    {{.Severity}}
                                </span>
                            </td>
                            <td>{{.ReasonCode}}</td>
                            <td>
                                <a href="/incidents/{{.ID}}" class="btn btn-sm btn-ghost">View</a>
                                <button class="btn btn-sm btn-ghost" hx-post="/incidents/{{.ID}}/replay" hx-target="#replay-result-{{.ID}}" hx-swap="innerHTML" hx-indicator="#replay-spinner-{{.ID}}">
                                    <i class="bi bi-arrow-repeat"></i>
                                </button>
                                <img id="replay-spinner-{{.ID}}" class="htmx-indicator" src="/spinner.svg" alt="loading" style="height:18px"/>
                            </td>
                        </tr>
                        {{else}}
                        <tr><td colspan="8" class="center-text muted">No incidents found</td></tr>
                        {{end}}
                    </tbody>
                </table>
            </div>
            {{else}}
            <div class="alert center-text">No incidents found</div>
            {{end}}
        </div>
    </div>
</main>
</body>
</html>
```

- [ ] **Step 2: Verify**

Run: `go test ./app/webapi/`
Expected: PASS for incident-list tests; no parse panic.

- [ ] **Step 3: Commit**

```bash
git add app/webapi/assets/incidents.html
git commit -m "feat(webui): restyle incidents page"
```

---

### Task 10: Convert `incident_detail.html`

**Files:**
- Modify (full rewrite): `app/webapi/assets/incident_detail.html`

- [ ] **Step 1: Replace `app/webapi/assets/incident_detail.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>Incident Detail - Shield</title>
    {{template "heads.html"}}
</head>
<body x-data="{ navOpen: false }">
{{template "sidebar.html"}}
<div class="scrim" x-show="navOpen" @click="navOpen=false" x-cloak></div>
<main class="main">
    <header class="topbar">
        <button class="topbar__burger" @click="navOpen=!navOpen" aria-label="Toggle navigation"><i class="bi bi-list"></i></button>
        <h2 class="page-title">Incident #{{.Incident.ID}}</h2>
    </header>
    <div class="content" id="incident-detail">
        <div class="flex between center mb-3">
            <span class="muted">Detection incident record</span>
            <a href="/incidents" class="btn btn-ghost btn-sm"><i class="bi bi-arrow-left"></i> Back to List</a>
        </div>

        <div class="grid-2-1">
            <div>
                <div class="card">
                    <div class="card-header">
                        <span>Details</span>
                        <span class="badge {{if eq .Incident.Status "open"}}badge-warn{{else if eq .Incident.Status "resolved"}}badge-ham{{else if eq .Incident.Status "appealed"}}badge-info{{else if eq .Incident.Status "reviewing"}}badge-info{{else}}badge-muted{{end}}">{{.Incident.Status}}</span>
                    </div>
                    <div class="card-body">
                        <table class="table">
                            <tr><th style="width:150px">Source</th><td><span class="badge badge-muted">{{.Incident.Source}}</span></td></tr>
                            <tr><th>Severity</th><td><span class="badge {{if eq .Incident.Severity "critical"}}badge-spam{{else if eq .Incident.Severity "high"}}badge-warn{{else if eq .Incident.Severity "medium"}}badge-info{{else}}badge-muted{{end}}">{{.Incident.Severity}}</span></td></tr>
                            <tr><th>Reason</th><td><code>{{.Incident.ReasonCode}}</code> &mdash; {{.Incident.ReasonText}}</td></tr>
                            <tr><th>Spam User</th><td>{{.Incident.SpamUserName}} <span class="muted">(ID: {{.Incident.SpamUserID}})</span></td></tr>
                            <tr><th>Chat ID</th><td>{{.Incident.ChatID}}</td></tr>
                            <tr><th>Created</th><td>{{.Incident.CreatedAt.Format "2006-01-02 15:04:05"}}</td></tr>
                            {{if .Incident.ResolvedAt}}<tr><th>Resolved</th><td>{{.Incident.ResolvedAt.Format "2006-01-02 15:04:05"}} by {{.Incident.ResolvedBy}}</td></tr>{{end}}
                            <tr><th>Idempotency Key</th><td><code>{{.Incident.IdempotencyKey}}</code></td></tr>
                        </table>
                    </div>
                </div>

                <div class="card">
                    <div class="card-header">Message</div>
                    <div class="card-body">
                        <pre style="white-space:pre-wrap">{{.Incident.MessageText}}</pre>
                        <div class="mt-2">
                            <button class="btn btn-ghost btn-sm" hx-post="/incidents/{{.Incident.ID}}/replay" hx-target="#replay-result" hx-swap="innerHTML" hx-indicator="#replay-spinner">
                                <i class="bi bi-arrow-repeat"></i> Replay Detection
                            </button>
                            <img id="replay-spinner" class="htmx-indicator" src="/spinner.svg" alt="Loading" style="height:20px"/>
                        </div>
                        <div id="replay-result" class="mt-2"></div>
                    </div>
                </div>

                <div class="card">
                    <div class="card-header">Timeline</div>
                    <div class="card-body">
                        <div class="table-wrap">
                            <table class="table">
                                <thead><tr><th>Time</th><th>Author</th><th>Action</th><th>Details</th></tr></thead>
                                <tbody>
                                {{range .Comments}}
                                    <tr>
                                        <td class="muted nowrap">{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
                                        <td><span class="badge badge-muted">{{.AuthorType}}</span> {{.AuthorID}}</td>
                                        <td>{{.Action}}</td>
                                        <td class="truncate">{{.Payload}}</td>
                                    </tr>
                                {{else}}
                                    <tr><td colspan="4" class="center-text muted">No comments yet</td></tr>
                                {{end}}
                                </tbody>
                            </table>
                        </div>
                    </div>
                </div>

                <div class="card">
                    <div class="card-header">Add Comment</div>
                    <div class="card-body">
                        <form hx-post="/incidents/{{.Incident.ID}}/comment" hx-target="#incident-detail" hx-swap="outerHTML" hx-error="#comment-error">
                            <input type="hidden" name="author_type" value="admin">
                            <input type="hidden" name="author_id" value="web-ui">
                            <div class="field">
                                <select class="select" name="action" style="max-width:200px">
                                    <option value="comment">Comment</option>
                                    <option value="note">Note</option>
                                    <option value="reviewed">Reviewed</option>
                                </select>
                            </div>
                            <div class="field">
                                <textarea class="input" name="payload" rows="2" placeholder="Comment text..."></textarea>
                            </div>
                            <button type="submit" class="btn btn-accent btn-sm">Add Comment</button>
                        </form>
                        <div id="comment-error" class="alert alert-danger hidden mt-2"></div>
                    </div>
                </div>
            </div>

            <div>
                <div class="card">
                    <div class="card-header">Actions</div>
                    <div class="card-body">
                        <form hx-post="/incidents/{{.Incident.ID}}/status" hx-target="#incident-detail" hx-swap="outerHTML" hx-error="#action-error">
                            <input type="hidden" name="resolved_by" value="web-ui">
                            <div class="flex flex-col gap-2">
                                {{if or (eq .Incident.Status "open") (eq .Incident.Status "reviewing") (eq .Incident.Status "appealed")}}
                                <button class="btn btn-ham btn-sm" name="status" value="resolved">Resolve</button>
                                <button class="btn btn-ghost btn-sm" name="status" value="closed">Close</button>
                                {{else}}
                                <span class="muted">No actions available</span>
                                {{end}}
                            </div>
                        </form>
                        <div id="action-error" class="alert alert-danger hidden mt-2"></div>
                    </div>
                </div>

                <div class="card">
                    <div class="card-header">Appeal</div>
                    <div class="card-body">
                        {{if .Appeal}}
                        <table class="table">
                            <tr><th>Status</th><td><span class="badge badge-info">{{.Appeal.Status}}</span></td></tr>
                            <tr><th>Appellant</th><td>{{.Appeal.AppellantName}}</td></tr>
                            <tr><th>Text</th><td>{{.Appeal.AppealText}}</td></tr>
                            {{if .Appeal.ResolvedAt}}<tr><th>Resolved</th><td>{{.Appeal.ResolvedAt.Format "2006-01-02 15:04:05"}}</td></tr>{{end}}
                        </table>
                        {{if or (eq .Appeal.Status "new") (eq .Appeal.Status "triaged")}}
                        <div class="flex flex-col gap-2 mt-2">
                            <form hx-post="/appeals/{{.Appeal.ID}}/resolve" hx-target="#incident-detail" hx-swap="outerHTML">
                                <input type="hidden" name="action" value="accept">
                                <input type="hidden" name="resolver_id" value="web-ui">
                                <button class="btn btn-ham btn-sm w-full">Accept Appeal</button>
                            </form>
                            <form hx-post="/appeals/{{.Appeal.ID}}/resolve" hx-target="#incident-detail" hx-swap="outerHTML">
                                <input type="hidden" name="action" value="reject">
                                <input type="hidden" name="resolver_id" value="web-ui">
                                <button class="btn btn-danger btn-sm w-full">Reject Appeal</button>
                            </form>
                        </div>
                        {{end}}
                        {{else}}
                        <span class="muted">No appeal filed</span>
                        {{end}}
                    </div>
                </div>
            </div>
        </div>
    </div>
</main>
</body>
</html>
```

Note: the original page-title was `Incident #{{.Incident.ID}}`; that moves into the `page-title` `<h2>`. The original status-button block repeated identical Resolve/Close buttons for the `open`, `reviewing`, and `appealed` branches — those are merged into one `{{if or ...}}` branch (same rendered output, DRY).

- [ ] **Step 2: Verify**

Run: `go test ./app/webapi/`
Expected: PASS for incident-detail tests; no parse panic.

- [ ] **Step 3: Commit**

```bash
git add app/webapi/assets/incident_detail.html
git commit -m "feat(webui): restyle incident detail page"
```

---

### Task 11: Convert `appeals.html`

**Files:**
- Modify (full rewrite): `app/webapi/assets/appeals.html`

- [ ] **Step 1: Replace `app/webapi/assets/appeals.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>Appeals - Shield</title>
    {{template "heads.html"}}
</head>
<body x-data="{ navOpen: false }">
{{template "sidebar.html"}}
<div class="scrim" x-show="navOpen" @click="navOpen=false" x-cloak></div>
<main class="main">
    <header class="topbar">
        <button class="topbar__burger" @click="navOpen=!navOpen" aria-label="Toggle navigation"><i class="bi bi-list"></i></button>
        <h2 class="page-title">Appeals Queue</h2>
    </header>
    <div class="content">
        <div class="field" style="max-width:280px">
            <label for="status-filter">Status</label>
            <select class="select" id="status-filter" hx-get="/appeals" hx-target="#appeals-content" hx-trigger="change" name="status">
                <option value="new" selected>New</option>
                <option value="triaged">Triaged</option>
                <option value="escalated">Escalated</option>
                <option value="accepted">Accepted</option>
                <option value="rejected">Rejected</option>
            </select>
        </div>

        <div id="appeals-content">
            {{template "appeals_list" .}}
        </div>
    </div>
</main>
</body>
</html>

{{define "appeals_list"}}
<div class="table-wrap">
    <table class="table">
        <thead>
            <tr>
                <th>ID</th>
                <th>Incident</th>
                <th>Appellant</th>
                <th>Status</th>
                <th>Text</th>
                <th>Created</th>
                <th>Actions</th>
            </tr>
        </thead>
        <tbody>
            {{range .}}
            <tr>
                <td><a href="/appeals/{{.ID}}">{{.ID}}</a></td>
                <td><a href="/incidents/{{.IncidentID}}">#{{.IncidentID}}</a></td>
                <td>{{.AppellantName}}</td>
                <td>
                    <span class="badge {{if eq .Status "new"}}badge-warn{{else if eq .Status "triaged"}}badge-info{{else if eq .Status "escalated"}}badge-spam{{else if eq .Status "accepted"}}badge-ham{{else}}badge-muted{{end}}">
                        {{.Status}}
                    </span>
                </td>
                <td>{{.AppealText}}</td>
                <td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
                <td><a href="/appeals/{{.ID}}" class="btn btn-sm btn-ghost">Review</a></td>
            </tr>
            {{else}}
            <tr>
                <td colspan="7" class="center-text muted">No appeals found</td>
            </tr>
            {{end}}
        </tbody>
    </table>
</div>
{{end}}
```

- [ ] **Step 2: Verify**

Run: `go test ./app/webapi/`
Expected: PASS for appeals tests; no parse panic.

- [ ] **Step 3: Commit**

```bash
git add app/webapi/assets/appeals.html
git commit -m "feat(webui): restyle appeals page"
```

---

### Task 12: Convert `feedback.html` (Alpine tabs)

**Files:**
- Modify (full rewrite): `app/webapi/assets/feedback.html`

The three `data-bs-toggle="tab"` panes become Alpine tabs (R3). The three `{{define}}` fragment sub-templates (`labels_list`, `candidates_list`, `knowledge_list`) only get the R2 class mapping — no Alpine, since they are HTMX-swapped table fragments.

- [ ] **Step 1: Replace `app/webapi/assets/feedback.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>Feedback - Shield</title>
    {{template "heads.html"}}
</head>
<body x-data="{ navOpen: false }">
{{template "sidebar.html"}}
<div class="scrim" x-show="navOpen" @click="navOpen=false" x-cloak></div>
<main class="main">
    <header class="topbar">
        <button class="topbar__burger" @click="navOpen=!navOpen" aria-label="Toggle navigation"><i class="bi bi-list"></i></button>
        <h2 class="page-title">Feedback</h2>
    </header>
    <div class="content" x-data="{ tab: 'labels' }">
        <div class="tabs">
            <button class="tab" :class="{ 'is-active': tab === 'labels' }" @click="tab='labels'">Labels</button>
            <button class="tab" :class="{ 'is-active': tab === 'candidates' }" @click="tab='candidates'">Candidates</button>
            <button class="tab" :class="{ 'is-active': tab === 'knowledge' }" @click="tab='knowledge'">Knowledge</button>
        </div>

        <div x-show="tab === 'labels'">
            <div class="filter-bar">
                <div class="field" style="margin-bottom:0">
                    <label>Label</label>
                    <select class="select" id="label-filter" hx-get="/api/feedback/labels" hx-target="#labels-content" hx-trigger="change" name="label">
                        <option value="">All</option>
                        <option value="confirmed_spam">Confirmed Spam</option>
                        <option value="false_positive">False Positive</option>
                        <option value="uncertain">Uncertain</option>
                    </select>
                </div>
                <div class="field" style="margin-bottom:0">
                    <label>Limit</label>
                    <select class="select" name="limit" hx-get="/api/feedback/labels" hx-target="#labels-content" hx-trigger="change">
                        <option value="25">25</option>
                        <option value="50" selected>50</option>
                        <option value="100">100</option>
                    </select>
                </div>
            </div>
            <div id="labels-content">
                {{template "labels_list" .Labels}}
            </div>
        </div>

        <div x-show="tab === 'candidates'" x-cloak>
            <div class="filter-bar">
                <div class="field" style="margin-bottom:0">
                    <label>Status</label>
                    <select class="select" id="candidate-status-filter" hx-get="/api/feedback/candidates" hx-target="#candidates-content" hx-trigger="change" name="status">
                        <option value="pending" selected>Pending</option>
                        <option value="">All</option>
                        <option value="approved">Approved</option>
                        <option value="rejected">Rejected</option>
                    </select>
                </div>
            </div>
            <div id="candidates-content">
                {{template "candidates_list" .Candidates}}
            </div>
        </div>

        <div x-show="tab === 'knowledge'" x-cloak>
            <div class="mb-3">
                <button class="btn btn-accent btn-sm" hx-post="/api/feedback/knowledge/snapshots" hx-target="#knowledge-content" hx-swap="innerHTML">
                    <i class="bi bi-camera"></i> Create Snapshot
                </button>
            </div>
            <div id="knowledge-content">
                {{template "knowledge_list" .Snapshots}}
            </div>
        </div>
    </div>
</main>
</body>
</html>

{{define "labels_list"}}
<div class="table-wrap">
    <table class="table">
        <thead>
            <tr>
                <th>ID</th>
                <th>Incident</th>
                <th>Label</th>
                <th>Source</th>
                <th>Labeled By</th>
                <th>Created</th>
            </tr>
        </thead>
        <tbody>
            {{range .}}
            <tr>
                <td>{{.ID}}</td>
                <td><a href="/incidents/{{.IncidentID}}">#{{.IncidentID}}</a></td>
                <td>
                    <span class="badge {{if eq .Label "confirmed_spam"}}badge-spam{{else if eq .Label "false_positive"}}badge-ham{{else}}badge-warn{{end}}">
                        {{.Label}}
                    </span>
                </td>
                <td><span class="badge badge-muted">{{.Source}}</span></td>
                <td>{{.LabeledBy}}</td>
                <td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
            </tr>
            {{else}}
            <tr><td colspan="6" class="center-text muted">No labels found</td></tr>
            {{end}}
        </tbody>
    </table>
</div>
{{end}}

{{define "candidates_list"}}
<div class="table-wrap">
    <table class="table">
        <thead>
            <tr>
                <th>ID</th>
                <th>Value</th>
                <th>Type</th>
                <th>Status</th>
                <th>Source</th>
                <th>Created</th>
                <th>Actions</th>
            </tr>
        </thead>
        <tbody>
            {{range .}}
            <tr id="candidate-{{.ID}}">
                <td>{{.ID}}</td>
                <td><code>{{.Value}}</code></td>
                <td><span class="badge badge-info">{{.Type}}</span></td>
                <td>
                    <span class="badge {{if eq .Status "pending"}}badge-warn{{else if eq .Status "approved"}}badge-ham{{else}}badge-muted{{end}}">
                        {{.Status}}
                    </span>
                </td>
                <td>{{.Source}}</td>
                <td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
                <td>
                    {{if eq .Status "pending"}}
                    <button class="btn btn-sm btn-ghost" hx-post="/api/feedback/candidates/{{.ID}}/approve" hx-target="#candidate-{{.ID}}" hx-swap="outerHTML">
                        <i class="bi bi-check-lg"></i>
                    </button>
                    <button class="btn btn-sm btn-ghost" hx-post="/api/feedback/candidates/{{.ID}}/reject" hx-target="#candidate-{{.ID}}" hx-swap="outerHTML">
                        <i class="bi bi-x-lg"></i>
                    </button>
                    {{end}}
                </td>
            </tr>
            {{else}}
            <tr><td colspan="7" class="center-text muted">No candidates found</td></tr>
            {{end}}
        </tbody>
    </table>
</div>
{{end}}

{{define "knowledge_list"}}
<div class="table-wrap">
    <table class="table">
        <thead>
            <tr>
                <th>ID</th>
                <th>Stop Phrases</th>
                <th>Spam Samples</th>
                <th>Ham Samples</th>
                <th>Dict Entries</th>
                <th>Created</th>
                <th>Actions</th>
            </tr>
        </thead>
        <tbody>
            {{range .}}
            <tr>
                <td>{{.ID}}</td>
                <td>{{len .Data.StopPhrases}}</td>
                <td>{{.Data.SpamSamples}}</td>
                <td>{{.Data.HamSamples}}</td>
                <td>{{.Data.DictionaryVer}}</td>
                <td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
                <td>
                    <button class="btn btn-sm btn-ghost" hx-post="/api/feedback/knowledge/snapshots/{{.ID}}/rollback"
                            hx-target="#knowledge-content" hx-swap="innerHTML" hx-confirm="Rollback to this snapshot?">
                        <i class="bi bi-arrow-counterclockwise"></i>
                    </button>
                </td>
            </tr>
            {{else}}
            <tr><td colspan="7" class="center-text muted">No snapshots found</td></tr>
            {{end}}
        </tbody>
    </table>
</div>
{{end}}
```

- [ ] **Step 2: Verify**

Run: `go test ./app/webapi/ -run TestFeedback`
Expected: PASS (webapi feedback handler tests assert status/content, not Bootstrap markup). No parse panic.

- [ ] **Step 3: Commit**

```bash
git add app/webapi/assets/feedback.html
git commit -m "feat(webui): restyle feedback page with Alpine tabs"
```

---

### Task 13: Convert `settings.html` and `dm_users.html` (Alpine tabs + collapse)

**Files:**
- Modify (full rewrite): `app/webapi/assets/settings.html`
- Modify (full rewrite): `app/webapi/assets/components/dm_users.html`

The 8 settings tabs become Alpine tabs (R3). The "find your ID" Bootstrap `collapse` becomes an Alpine `x-show`. The inline `copyUserID` `<script>` is removed — `copyUserID` now lives in `app.js` (Task 2).

- [ ] **Step 1: Replace `app/webapi/assets/settings.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>Settings - Shield</title>
    {{template "heads.html"}}
</head>
<body x-data="{ navOpen: false }">
{{template "sidebar.html"}}
<div class="scrim" x-show="navOpen" @click="navOpen=false" x-cloak></div>
<main class="main">
    <header class="topbar">
        <button class="topbar__burger" @click="navOpen=!navOpen" aria-label="Toggle navigation"><i class="bi bi-list"></i></button>
        <h2 class="page-title">Application Settings</h2>
    </header>
    <div class="content">
        <div class="mb-3">
            <a href="/settings/edit" class="btn btn-accent btn-sm">Edit Settings</a>
        </div>

        <div class="grid-2-1 mb-4">
            <div class="card">
                <div class="card-header">System Status</div>
                <div class="card-body">
                    <div class="grid-2">
                        <table class="table">
                            <thead><tr><th colspan="2">Telegram Bot</th></tr></thead>
                            <tbody>
                                <tr><th style="width:40%">Version</th><td>{{.Version}}</td></tr>
                                <tr><th>Uptime</th><td>{{.System.Uptime}}</td></tr>
                                <tr><th>Tenant ID</th><td>{{.TenantID}}</td></tr>
                                <tr><th>Primary Group</th><td>{{.PrimaryGroup}}</td></tr>
                                <tr><th>Admin Group</th><td>{{.AdminGroup}}</td></tr>
                            </tbody>
                        </table>
                        <table class="table">
                            <thead><tr><th colspan="2">Database</th></tr></thead>
                            <tbody>
                                <tr><th style="width:40%">Type</th><td>{{.Database.Type}}</td></tr>
                                <tr><th>Tenant ID</th><td>{{.Database.TenantID}}</td></tr>
                                <tr><th>Status</th><td>
                                    {{if eq .Database.Status "Connected"}}
                                        <span class="badge badge-ham">{{.Database.Status}}</span>
                                    {{else}}
                                        <span class="badge badge-spam">{{.Database.Status}}</span>
                                    {{end}}
                                </td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>

            <div class="card">
                <div class="card-header">Backup &amp; Recovery</div>
                <div class="card-body">
                    <div class="flex flex-col gap-2">
                        <a href="{{.Backup.URL}}" class="btn btn-ghost" download="{{.Backup.Filename}}">
                            <i class="bi bi-download"></i> Download Database Backup
                        </a>
                        {{if eq .Database.Type "sqlite"}}
                        <a href="/download/export-to-postgres" class="btn btn-ghost">
                            <i class="bi bi-download"></i> Export to PostgreSQL
                        </a>
                        {{end}}
                    </div>
                    <p class="small muted mt-2">Store backup files in a safe location.</p>
                </div>
            </div>
        </div>

        <div x-data="{ tab: 'spam-detection' }">
            <div class="tabs">
                <button class="tab" :class="{ 'is-active': tab === 'spam-detection' }" @click="tab='spam-detection'"><i class="bi bi-shield-check"></i> Spam Detection</button>
                <button class="tab" :class="{ 'is-active': tab === 'meta-checks' }" @click="tab='meta-checks'"><i class="bi bi-check-circle"></i> Meta Checks</button>
                <button class="tab" :class="{ 'is-active': tab === 'openai' }" @click="tab='openai'"><i class="bi bi-chat-square-text"></i> OpenAI</button>
                <button class="tab" :class="{ 'is-active': tab === 'gemini' }" @click="tab='gemini'"><i class="bi bi-stars"></i> Gemini</button>
                <button class="tab" :class="{ 'is-active': tab === 'lua-plugins' }" @click="tab='lua-plugins'"><i class="bi bi-code-square"></i> Lua Plugins</button>
                <button class="tab" :class="{ 'is-active': tab === 'data-storage' }" @click="tab='data-storage'"><i class="bi bi-hdd-stack"></i> Data Storage</button>
                <button class="tab" :class="{ 'is-active': tab === 'behavior' }" @click="tab='behavior'"><i class="bi bi-gear"></i> Bot Behavior</button>
                <button class="tab" :class="{ 'is-active': tab === 'system' }" @click="tab='system'"><i class="bi bi-tools"></i> System</button>
            </div>

            <div x-show="tab === 'spam-detection'">
                <div class="table-wrap">
                    <table class="table">
                        <thead><tr><th colspan="2">Spam Detection Settings</th></tr></thead>
                        <tbody>
                            <tr><th style="width:30%">Similarity Threshold</th><td>{{.SimilarityThreshold}}</td></tr>
                            <tr><th>Min Message Length</th><td>{{.MinMsgLen}}</td></tr>
                            <tr><th>Max Emoji</th><td>{{.MaxEmoji}}</td></tr>
                            <tr><th>Min Spam Probability</th><td>{{.MinSpamProbability}}</td></tr>
                            <tr><th>First Messages Count</th><td>{{.FirstMessagesCount}}</td></tr>
                            <tr><th>Multi Lingual Words</th><td>{{.MultiLangLimit}}</td></tr>
                            <tr><th>LLM Consensus</th><td>{{.LLMConsensus}}</td></tr>
                            <tr><th>Abnormal Spacing Enabled</th><td>{{.AbnormalSpacingEnabled}}</td></tr>
                            <tr><th>History Size</th><td>{{.HistorySize}}</td></tr>
                            <tr><th>Paranoid Mode</th><td>{{.ParanoidMode}}</td></tr>
                            <tr><th>Forward Prohibited</th><td>{{.MetaForwarded}}</td></tr>
                            <tr><th>CAS Enabled</th><td>{{.CasEnabled}}</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <div x-show="tab === 'meta-checks'" x-cloak>
                <div class="table-wrap">
                    <table class="table">
                        <thead><tr><th colspan="2">Meta Checks Settings</th></tr></thead>
                        <tbody>
                            <tr><th style="width:30%">Meta Enabled</th><td>{{.MetaEnabled}}</td></tr>
                            <tr><th>Meta Links Limit</th><td>{{if eq .MetaLinksLimit -1}}disabled{{else}}{{.MetaLinksLimit}}{{end}}</td></tr>
                            <tr><th>Meta Mentions Limit</th><td>{{if eq .MetaMentionsLimit -1}}disabled{{else}}{{.MetaMentionsLimit}}{{end}}</td></tr>
                            <tr><th>Meta Links Only</th><td>{{.MetaLinksOnly}}</td></tr>
                            <tr><th>Meta Image Only</th><td>{{.MetaImageOnly}}</td></tr>
                            <tr><th>Meta Video Only</th><td>{{.MetaVideoOnly}}</td></tr>
                            <tr><th>Meta Audio Only</th><td>{{.MetaAudioOnly}}</td></tr>
                            <tr><th>Meta Keyboard</th><td>{{.MetaKeyboard}}</td></tr>
                            <tr><th>Meta Contact Only</th><td>{{.MetaContactOnly}}</td></tr>
                            <tr><th>Meta Username Symbols</th><td>{{if eq .MetaUsernameSymbols ""}}disabled{{else}}{{.MetaUsernameSymbols}}{{end}}</td></tr>
                            <tr><th>Meta Giveaway</th><td>{{.MetaGiveaway}}</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <div x-show="tab === 'openai'" x-cloak>
                <div class="table-wrap">
                    <table class="table">
                        <thead><tr><th colspan="2">OpenAI Integration Settings</th></tr></thead>
                        <tbody>
                            <tr><th style="width:30%">OpenAI Enabled</th><td>{{.OpenAIEnabled}}</td></tr>
                            <tr><th>OpenAI Veto</th><td>{{.OpenAIVeto}}</td></tr>
                            <tr><th>OpenAI History Size</th><td>{{.OpenAIHistorySize}}</td></tr>
                            <tr><th>OpenAI Model</th><td>{{.OpenAIModel}}</td></tr>
                            <tr><th>Check Short Messages</th><td>{{.OpenAICheckShortMessages}}</td></tr>
                            <tr><th>Custom Prompts</th><td>
                                {{if eq (len .OpenAICustomPrompts) 0}}None configured{{else}}{{range .OpenAICustomPrompts}}{{.}}<br>{{end}}{{end}}
                            </td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <div x-show="tab === 'gemini'" x-cloak>
                <div class="table-wrap">
                    <table class="table">
                        <thead><tr><th colspan="2">Gemini Integration Settings</th></tr></thead>
                        <tbody>
                            <tr><th style="width:30%">Gemini Enabled</th><td>{{.GeminiEnabled}}</td></tr>
                            <tr><th>Gemini Veto</th><td>{{.GeminiVeto}}</td></tr>
                            <tr><th>Gemini History Size</th><td>{{.GeminiHistorySize}}</td></tr>
                            <tr><th>Gemini Model</th><td>{{.GeminiModel}}</td></tr>
                            <tr><th>Check Short Messages</th><td>{{.GeminiCheckShortMessages}}</td></tr>
                            <tr><th>Custom Prompts</th><td>
                                {{if eq (len .GeminiCustomPrompts) 0}}None configured{{else}}{{range .GeminiCustomPrompts}}{{.}}<br>{{end}}{{end}}
                            </td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <div x-show="tab === 'lua-plugins'" x-cloak>
                <div class="table-wrap">
                    <table class="table">
                        <thead><tr><th colspan="2">Lua Plugins Settings</th></tr></thead>
                        <tbody>
                            <tr><th style="width:30%">Lua Plugins Enabled</th><td>{{.LuaPluginsEnabled}}</td></tr>
                            <tr><th>Plugins Directory</th><td>{{if eq .LuaPluginsDir ""}}Not set{{else}}{{.LuaPluginsDir}}{{end}}</td></tr>
                            <tr><th>Dynamic Reload</th><td>{{.LuaDynamicReload}}</td></tr>
                            <tr><th>Enabled Plugins</th><td>
                                {{if and .LuaPluginsEnabled (eq (len .LuaEnabledPlugins) 0)}}All plugins are enabled
                                {{else if .LuaPluginsEnabled}}{{range .LuaEnabledPlugins}}{{.}}<br>{{end}}
                                {{else}}Plugins disabled{{end}}
                            </td></tr>
                            <tr><th>Available Plugins</th><td>
                                {{if .LuaPluginsEnabled}}
                                    {{if eq (len .LuaAvailablePlugins) 0}}No plugins available
                                    {{else}}{{range .LuaAvailablePlugins}}{{.}}<br>{{end}}{{end}}
                                {{else}}Plugins disabled{{end}}
                            </td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <div x-show="tab === 'data-storage'" x-cloak>
                <div class="table-wrap">
                    <table class="table">
                        <thead><tr><th colspan="2">Data Storage Settings</th></tr></thead>
                        <tbody>
                            <tr><th style="width:30%">Samples Data Path</th><td>{{.SamplesDataPath}}</td></tr>
                            <tr><th>Dynamic Data Path</th><td>{{.DynamicDataPath}}</td></tr>
                            <tr><th>Watch Interval Seconds</th><td>{{.WatchIntervalSecs}}</td></tr>
                            <tr><th>Storage Timeout</th><td>{{.StorageTimeout}}</td></tr>
                            <tr><th>Training Enabled</th><td>{{.TrainingEnabled}}</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <div x-show="tab === 'behavior'" x-cloak>
                <div class="table-wrap">
                    <table class="table">
                        <thead><tr><th colspan="2">Bot Behavior Settings</th></tr></thead>
                        <tbody>
                            <tr><th style="width:30%">Super Users</th><td>{{range .SuperUsers}}{{.}}<br>{{end}}</td></tr>
                            <tr><th>Disable Admin Spam Forward</th><td>{{.DisableAdminSpamForward}}</td></tr>
                            <tr><th>No Spam Reply</th><td>{{.NoSpamReply}}</td></tr>
                            <tr><th>Startup Message Enabled</th><td>{{.StartupMessageEnabled}}</td></tr>
                            <tr><th>Soft Ban Enabled</th><td>{{.SoftBanEnabled}}</td></tr>
                        </tbody>
                    </table>
                </div>

                <div class="mt-3" x-data="{ show: false }">
                    <button class="btn btn-ghost" type="button" @click="show = !show">
                        <i class="bi bi-info-circle"></i> Don't know your ID? Message the bot!
                    </button>
                    <div class="card mt-2" id="dm-users-panel" x-show="show" x-cloak>
                        <div class="card-body">
                            <p class="mb-2"><strong>How to find your Telegram User ID:</strong></p>
                            <ol class="mb-3">
                                <li>Open a chat with the bot: {{if .BotUsername}}<a href="https://t.me/{{.BotUsername}}" target="_blank">t.me/{{.BotUsername}}</a>{{else}}find the bot in Telegram{{end}}</li>
                                <li>Send any message</li>
                                <li>Click <strong>Refresh</strong> below — your ID will appear in the table</li>
                            </ol>
                            <div id="dm-users-container" hx-get="/dm-users" hx-trigger="intersect once" hx-swap="innerHTML">
                                <div class="center-text muted">Loading...</div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <div x-show="tab === 'system'" x-cloak>
                <div class="table-wrap">
                    <table class="table">
                        <thead><tr><th colspan="2">System Settings</th></tr></thead>
                        <tbody>
                            <tr><th style="width:30%">Logger Enabled</th><td>{{.LoggerEnabled}}</td></tr>
                            <tr><th>Debug Mode Enabled</th><td>{{.DebugModeEnabled}}</td></tr>
                            <tr><th>Dry Mode Enabled</th><td>{{.DryModeEnabled}}</td></tr>
                            <tr><th>TG Debug Mode Enabled</th><td>{{.TGDebugModeEnabled}}</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    </div>
</main>
</body>
</html>
```

- [ ] **Step 2: Replace `app/webapi/assets/components/dm_users.html`**

```html
{{define "dm_users.html"}}
<div class="flex end mb-2">
    <button class="btn btn-sm btn-ghost" hx-get="/dm-users" hx-target="#dm-users-container" hx-swap="innerHTML">
        <i class="bi bi-arrow-clockwise"></i> Refresh
    </button>
</div>
<div class="table-wrap">
    <table class="table">
        <thead>
            <tr>
                <th>ID</th>
                <th>Display Name</th>
                <th>Username</th>
                <th>When</th>
                <th></th>
            </tr>
        </thead>
        <tbody>
            {{range .Users}}
            <tr>
                <td>{{.UserID}}</td>
                <td>{{.DisplayName}}</td>
                <td>{{if .UserName}}@{{.UserName}}{{end}}</td>
                <td>{{.When}}</td>
                <td><button class="btn btn-sm btn-ghost" onclick="copyUserID({{.UserID}}, this)">Copy ID</button></td>
            </tr>
            {{else}}
            <tr>
                <td colspan="5" class="muted center-text">No recent DM users. Send a message to the bot, then click Refresh.</td>
            </tr>
            {{end}}
        </tbody>
    </table>
</div>
{{end}}
```

- [ ] **Step 3: Verify**

Run: `go test ./app/webapi/ -run "TestServer_htmlSettingsHandler|TestDMUsers"`
Expected: FAIL — `TestDMUsers_settingsPageContainsDMUsersSection` (`webapi_part9_test.go:78-79,89-90`) still asserts `data-bs-toggle="collapse"` and the inline `copyUserID` script. Fixed in Task 16. Confirm there is no template parse panic and `TestServer_htmlSettingsHandler` passes.

- [ ] **Step 4: Commit**

```bash
git add app/webapi/assets/settings.html app/webapi/assets/components/dm_users.html
git commit -m "feat(webui): restyle settings page with Alpine tabs"
```

---

### Task 14: Convert `settings_edit.html`

**Files:**
- Modify (full rewrite): `app/webapi/assets/settings_edit.html`

The page keeps the `dict` FuncMap helper and the `{{template "settings_*"}}` field sub-templates. The `mb-3 row` / `col-sm-4` / `col-sm-8` field layout becomes `.form-row`. The pinned badge keeps its `env-pinned` text (asserted by `handlers_settings_test.go:54`).

- [ ] **Step 1: Replace `app/webapi/assets/settings_edit.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>Edit Settings - Shield</title>
    {{template "heads.html"}}
</head>
<body x-data="{ navOpen: false }">
{{template "sidebar.html"}}
<div class="scrim" x-show="navOpen" @click="navOpen=false" x-cloak></div>
<main class="main">
    <header class="topbar">
        <button class="topbar__burger" @click="navOpen=!navOpen" aria-label="Toggle navigation"><i class="bi bi-list"></i></button>
        <h2 class="page-title">Edit Settings</h2>
    </header>
    <div class="content">
        <div id="settings-error"></div>
        <div id="settings-status"></div>

        <form hx-post="/settings/save" hx-target="#settings-status" hx-swap="innerHTML">
            {{$pinned := .EnvPinned}}

            <div class="card">
                <div class="card-header">Detection</div>
                <div class="card-body">
                    {{template "settings_int" dict "Key" "detection.max_emoji" "Label" "Max emoji" "Val" .RuleSet.Detection.MaxEmoji "Pinned" $pinned}}
                    {{template "settings_int" dict "Key" "detection.min_msg_len" "Label" "Min message length" "Val" .RuleSet.Detection.MinMsgLen "Pinned" $pinned}}
                    {{template "settings_float" dict "Key" "detection.similarity_threshold" "Label" "Similarity threshold" "Val" .RuleSet.Detection.SimilarityThreshold "Pinned" $pinned}}
                    {{template "settings_float" dict "Key" "detection.min_spam_probability" "Label" "Min spam probability %" "Val" .RuleSet.Detection.MinSpamProbability "Pinned" $pinned}}
                    {{template "settings_int" dict "Key" "detection.multi_lang_words" "Label" "Multi-lang words" "Val" .RuleSet.Detection.MultiLangWords "Pinned" $pinned}}
                    {{template "settings_int" dict "Key" "detection.history_size" "Label" "History size" "Val" .RuleSet.Detection.HistorySize "Pinned" $pinned}}
                    {{template "settings_int" dict "Key" "detection.first_messages_count" "Label" "First messages count" "Val" .RuleSet.Detection.FirstMessagesCount "Pinned" $pinned}}
                    {{template "settings_bool" dict "Key" "detection.cas_enabled" "Label" "CAS enabled" "Val" .RuleSet.Detection.CasEnabled "Pinned" $pinned}}
                    {{template "settings_bool" dict "Key" "detection.paranoid_mode" "Label" "Paranoid mode" "Val" .RuleSet.Detection.ParanoidMode "Pinned" $pinned}}
                </div>
            </div>

            <div class="card">
                <div class="card-header">LLM</div>
                <div class="card-body">
                    {{template "settings_enum" dict "Key" "llm.mode" "Label" "LLM mode" "Val" .RuleSet.LLM.Mode "Options" .LLMModes "Pinned" $pinned}}
                    {{template "settings_enum" dict "Key" "llm.consensus" "Label" "LLM consensus" "Val" .RuleSet.LLM.Consensus "Options" .LLMConsensus "Pinned" $pinned}}
                    {{template "settings_text" dict "Key" "llm.vision_prompt" "Label" "Vision prompt" "Val" .RuleSet.LLM.VisionPrompt "Pinned" $pinned}}
                </div>
            </div>

            <div class="card">
                <div class="card-header">OpenAI</div>
                <div class="card-body">
                    {{template "settings_bool" dict "Key" "openai.veto" "Label" "Veto" "Val" .RuleSet.OpenAI.Veto "Pinned" $pinned}}
                    {{template "settings_bool" dict "Key" "openai.check_short_messages" "Label" "Check short messages" "Val" .RuleSet.OpenAI.CheckShortMessages "Pinned" $pinned}}
                    {{template "settings_str" dict "Key" "openai.model" "Label" "Model" "Val" .RuleSet.OpenAI.Model "Pinned" $pinned}}
                    {{template "settings_int" dict "Key" "openai.history_size" "Label" "History size" "Val" .RuleSet.OpenAI.HistorySize "Pinned" $pinned}}
                    {{template "settings_text" dict "Key" "openai.prompt" "Label" "System prompt" "Val" .RuleSet.OpenAI.Prompt "Pinned" $pinned}}
                </div>
            </div>

            <div class="card">
                <div class="card-header">Gemini</div>
                <div class="card-body">
                    {{template "settings_bool" dict "Key" "gemini.veto" "Label" "Veto" "Val" .RuleSet.Gemini.Veto "Pinned" $pinned}}
                    {{template "settings_bool" dict "Key" "gemini.check_short_messages" "Label" "Check short messages" "Val" .RuleSet.Gemini.CheckShortMessages "Pinned" $pinned}}
                    {{template "settings_str" dict "Key" "gemini.model" "Label" "Model" "Val" .RuleSet.Gemini.Model "Pinned" $pinned}}
                    {{template "settings_int" dict "Key" "gemini.history_size" "Label" "History size" "Val" .RuleSet.Gemini.HistorySize "Pinned" $pinned}}
                    {{template "settings_text" dict "Key" "gemini.prompt" "Label" "System prompt" "Val" .RuleSet.Gemini.Prompt "Pinned" $pinned}}
                </div>
            </div>

            <div class="card">
                <div class="card-body">
                    {{template "settings_bool" dict "Key" "slow_path_enabled" "Label" "Slow-path enabled" "Val" .RuleSet.SlowPathEnabled "Pinned" $pinned}}
                </div>
            </div>

            <button type="submit" class="btn btn-accent">Save</button>
        </form>
    </div>
</main>
</body>
</html>

{{define "settings_pinned_badge"}}
{{if index .Pinned .Key}}<span class="badge badge-warn" title="Set by env var; this will be overwritten on restart. Remove it from env to manage it here.">env-pinned</span>{{end}}
{{end}}

{{define "settings_int"}}
<div class="form-row">
    <label>{{.Label}} {{template "settings_pinned_badge" .}}</label>
    <div class="form-row__control">
        <input type="number" step="1" class="input" name="{{.Key}}" value="{{.Val}}">
    </div>
</div>
{{end}}

{{define "settings_float"}}
<div class="form-row">
    <label>{{.Label}} {{template "settings_pinned_badge" .}}</label>
    <div class="form-row__control">
        <input type="number" step="any" class="input" name="{{.Key}}" value="{{.Val}}">
    </div>
</div>
{{end}}

{{define "settings_str"}}
<div class="form-row">
    <label>{{.Label}} {{template "settings_pinned_badge" .}}</label>
    <div class="form-row__control">
        <input type="text" class="input" name="{{.Key}}" value="{{.Val}}">
    </div>
</div>
{{end}}

{{define "settings_text"}}
<div class="form-row">
    <label>{{.Label}} {{template "settings_pinned_badge" .}}</label>
    <div class="form-row__control">
        <textarea class="input" name="{{.Key}}" rows="3">{{.Val}}</textarea>
    </div>
</div>
{{end}}

{{define "settings_bool"}}
<div class="form-row">
    <label>{{.Label}} {{template "settings_pinned_badge" .}}</label>
    <div class="form-row__control">
        <input type="checkbox" class="checkbox" name="{{.Key}}" {{if .Val}}checked{{end}}>
    </div>
</div>
{{end}}

{{define "settings_enum"}}
<div class="form-row">
    <label>{{.Label}} {{template "settings_pinned_badge" .}}</label>
    <div class="form-row__control">
        <select class="select" name="{{.Key}}">
            {{$cur := .Val}}
            {{range .Options}}<option value="{{.}}" {{if eq . $cur}}selected{{end}}>{{if eq . ""}}(default){{else}}{{.}}{{end}}</option>{{end}}
        </select>
    </div>
</div>
{{end}}
```

- [ ] **Step 2: Verify**

Run: `go test ./app/webapi/ -run "TestHTMLSettingsEditHandler|TestSaveSettingsHandler"`
Expected: PASS (the `env-pinned` badge text is preserved; `handlers_settings_test.go:54` still matches).

- [ ] **Step 3: Commit**

```bash
git add app/webapi/assets/settings_edit.html
git commit -m "feat(webui): restyle edit-settings page"
```

---

### Task 15: Remove the obsolete navbar component

**Files:**
- Delete: `app/webapi/assets/components/navbar.html`

- [ ] **Step 1: Confirm no template references `navbar.html`**

Run: `grep -rn "navbar.html" app/webapi/assets`
Expected: no output (all 11 pages now use `sidebar.html`).

- [ ] **Step 2: Delete the file**

```bash
git rm app/webapi/assets/components/navbar.html
```

- [ ] **Step 3: Verify the test suite**

Run: `go test ./app/webapi/`
Expected: same failures as before (the Task 16 assertion failures only) — confirm no new failure and no parse panic. `navbar.html` removal must not break `ParseFS`.

- [ ] **Step 4: Commit**

```bash
git commit -m "chore(webui): remove obsolete navbar component"
```

---

### Task 16: Update webapi unit-test assertions

**Files:**
- Modify: `app/webapi/webapi_part3_test.go:335`
- Modify: `app/webapi/webapi_part4_test.go:71,140,141,165,314,345`
- Modify: `app/webapi/webapi_part9_test.go:55-91`

- [ ] **Step 1: Fix the download-button class assertion in `webapi_part3_test.go`**

At `webapi_part3_test.go:335`, replace:

```go
		assert.Contains(t, body, "btn-custom-blue")
```

with:

```go
		assert.Contains(t, body, "btn-ghost")
```

- [ ] **Step 2: Fix the title and samples-list assertions in `webapi_part4_test.go`**

Replace each of these five title assertions (lines 71, 140, 165, 314, 345) — change `TG-Spam` to `Shield`:

```go
		assert.Contains(t, body, "<title>Checker - Shield</title>", "template should contain the correct title")
```
```go
	assert.Contains(t, body, "<title>Manage Samples - Shield</title>", "template should contain the correct title")
```
```go
		assert.Contains(t, body, "<title>Manage Users - Shield</title>", "template should contain the correct title")
```
```go
		assert.Contains(t, body, "<title>Settings - Shield</title>", "template should contain the correct title")
```
```go
		assert.Contains(t, body, "<title>Settings - Shield</title>", "template should contain the correct title")
```

And at `webapi_part4_test.go:141`, replace the samples-list markup assertion:

```go
	assert.Contains(t, body, `<div class="row" id="samples-list">`, "template should contain a samples list")
```

with:

```go
	assert.Contains(t, body, `id="samples-list"`, "template should contain a samples list")
```

- [ ] **Step 3: Fix the DM-users settings-section test in `webapi_part9_test.go`**

Replace the body of `TestDMUsers_settingsPageContainsDMUsersSection` (lines 76-90) — the panel id, instructional text, and `hx-get` assertions stay; the Bootstrap `data-bs-*` collapse assertions become Alpine assertions, and the inline-`copyUserID` assertions are dropped (`copyUserID` now lives in the static `app.js`):

```go
	assert.Contains(t, body, `id="dm-users-panel"`)
	assert.Contains(t, body, "Don't know your ID? Message the bot!")
	assert.Contains(t, body, `x-show="show"`)
	assert.Contains(t, body, `@click="show = !show"`)

	assert.Contains(t, body, "How to find your Telegram User ID")
	assert.Contains(t, body, "Open a chat with the bot")
	assert.Contains(t, body, "Send any message")
	assert.Contains(t, body, "your ID will appear in the table")

	assert.Contains(t, body, `hx-get="/dm-users"`)
	assert.NotContains(t, body, `sse-connect="/dm-users/stream"`, "SSE should not be in settings page")

	assert.Contains(t, body, "app.js")
}
```

- [ ] **Step 4: Run the full webapi test suite**

Run: `go test ./app/webapi/`
Expected: PASS — all assertions now match the restyled markup.

- [ ] **Step 5: Commit**

```bash
git add app/webapi/webapi_part3_test.go app/webapi/webapi_part4_test.go app/webapi/webapi_part9_test.go
git commit -m "test(webui): update assertions for restyled markup"
```

---

### Task 17: Update e2e Playwright tests

**Files:**
- Modify: `e2e-ui/e2e_test.go:184,263-265,276,329`

- [ ] **Step 1: Fix the page-title assertion in `TestChecker_PageLoads`**

At `e2e-ui/e2e_test.go:184`, replace:

```go
	assert.Contains(t, title, "TG-Spam")
```

with:

```go
	assert.Contains(t, title, "Shield")
```

- [ ] **Step 2: Fix the feedback tab selectors in `TestFeedback_PageLoads`**

Replace the three tab locators (lines 263-265):

```go
	waitVisible(t, page.Locator("button.tab:has-text('Labels')"))
	waitVisible(t, page.Locator("button.tab:has-text('Candidates')"))
	waitVisible(t, page.Locator("button.tab:has-text('Knowledge')"))
```

- [ ] **Step 3: Fix the sidebar selector in `TestNavbar_NavigationWorks`**

At `e2e-ui/e2e_test.go:276`, replace:

```go
	waitVisible(t, page.Locator(".navbar"))
```

with:

```go
	waitVisible(t, page.Locator(".sidebar"))
```

(The navigation links keep the `.nav-link` class, so the loop at lines 292-298 needs no change.)

- [ ] **Step 4: Fix the result-container selector in `TestChecker_CheckMessage`**

At `e2e-ui/e2e_test.go:329`, replace:

```go
	result := page.Locator("#result .alert-light")
```

with:

```go
	result := page.Locator("#result .alert")
```

- [ ] **Step 5: Verify the e2e suite compiles**

Run: `go vet -tags e2e ./e2e-ui/`
Expected: no errors. (The full e2e run requires Playwright browsers and a built binary; CI runs it. A local compile-check via `go vet -tags e2e` is sufficient for this task.)

- [ ] **Step 6: Commit**

```bash
git add e2e-ui/e2e_test.go
git commit -m "test(webui): update e2e selectors for restyled UI"
```

---

### Task 18: Final verification

**Files:**
- Modify (only if a stale UI reference is found): `README.md`

- [ ] **Step 1: Run the full test suite with the race detector**

Run: `go test -race ./...`
Expected: PASS.

- [ ] **Step 2: Run the linter**

Run: `golangci-lint run`
Expected: no findings. (Only `routes.go` and the three test files changed in Go; fix any finding inline.)

- [ ] **Step 3: Normalize comments**

Run: `command -v unfuck-ai-comments >/dev/null || go install github.com/umputun/unfuck-ai-comments@latest; unfuck-ai-comments run --fmt --skip=mocks ./...`
Expected: no changes, or only trivial comment fixes. If it changes files, `git add` and amend them into the relevant commit or make a follow-up `chore` commit.

- [ ] **Step 4: Scan README for stale UI references**

Run: `grep -niE "bootstrap|navbar|tg-spam" README.md`
Inspect the matches. If the README describes the UI as Bootstrap-based or references a top navbar, update that prose to describe the dark ops-console sidebar UI. If the matches are only the `tg-spam` project name / binary name (unrelated to the restyle), leave them. Do not rename the project.

- [ ] **Step 5: Commit any README change**

```bash
git add README.md
git commit -m "docs: update UI description for the restyle"
```

(Skip this step if Step 4 found nothing to change.)

- [ ] **Step 6: Final confirmation**

Run: `go build ./... && go test ./app/webapi/`
Expected: PASS. The restyle branch is complete.

---

## Self-Review

**1. Spec coverage** (against `2026-05-16-ui-restyle-design.md`):

- Drop Bootstrap CSS/JS → Task 2 (heads.html). ✓
- Own hand-written CSS, tokens + components, single `styles.css`, no build → Task 1. ✓
- Dark ops-console theme → Task 1 tokens. ✓
- Alpine.js via CDN → Task 2; used in Tasks 4-14 (navOpen), 12 & 13 (tabs), 13 (collapse). ✓
- Top navbar → left sidebar → Task 3 (sidebar.html), Task 15 (delete navbar.html). ✓
- Minecraft flavor (pixel fonts, blocky radii, beveled buttons, MC palette) → Task 1 (`--font-brand`/`--font-head`, `--radius:2px`, `.btn` bevel `box-shadow`, `--spam`/`--ham`/`--gold`). ✓
- Brand "Shield" → Task 3 (`sidebar__brand-name`), titles changed in every page task. ✓
- Keep HTMX / embed.FS / server-rendered templates / bootstrap-icons → preserved throughout; HTMX pinned in Task 2. ✓
- All 11 pages + 3 components re-classed → Tasks 4-14 (11 pages), Tasks 2/3/13 (heads, sidebar, dm_users). ✓
- HTMX + Alpine init on swapped content → Task 2 `app.js` `htmx:afterSwap` → `Alpine.initTree`. ✓
- e2e tests updated as mandatory work → Task 17. ✓
- Per-page template-parse + handler-200 gate → every page task Step 2. ✓
- Single branch, merges whole → all tasks on `feat/ui-restyle`. ✓

No spec gaps. The spec mentions modals "if any" — investigation found `incident_detail.html` and `appeals.html` use no modals, so no modal component is built (YAGNI). The spec mentions a "user dropdown"; the spec's own layout section specifies a flat "user block at the bottom" — Task 3 implements flat footer links, so no dropdown component is needed.

**2. Placeholder scan:** No "TBD"/"TODO"/"handle edge cases"/"similar to Task N". Every file step shows complete content; every command step shows the exact command and expected result.

**3. Type/identifier consistency:** Class names are consistent across CSS (Task 1) and all templates — `.sidebar`/`.is-open`, `.nav-link`/`.is-active`, `.btn`+`.btn-accent`/`.btn-danger`/`.btn-ham`/`.btn-warn`/`.btn-ghost`/`.btn-sm`, `.card`/`.card-header`/`.card-body`, `.table`/`.table-wrap`, `.input`/`.select`/`.checkbox`/`.field`/`.form-row`/`.form-row__control`, `.alert`+`-danger`/`-success`/`-warn`/`-info`, `.badge`+`-spam`/`-ham`/`-warn`/`-info`/`-muted`, `.tabs`/`.tab`/`.is-active`, `.list`/`.list-item`, `.grid-2`/`.grid-2-1`/`.filter-bar`, `.hidden`/`.only-mobile`/`.only-desktop`, `.scrim`, `.topbar`/`.topbar__burger`/`.page-title`/`.content`/`.main`. Alpine state names are consistent: `navOpen` (body scope), `tab` (feedback/settings tab scope), `show` (settings collapse scope). The `htmx:afterSwap` handler and `copyUserID` are defined once in `app.js`.
