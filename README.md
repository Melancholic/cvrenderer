# cvrenderer

A small, self-hosted HTTP service that renders your CV to **PDF** from a YAML
file you own. Written in Go, using [Typst](https://typst.app) as the rendering
engine. No external services, no accounts, no scraping — hit an endpoint, get a
PDF.

## How it works

```
GET /cv/<template>/<name>.pdf ─▶ Go service ─▶ typst compile ─▶ application/pdf
                                                    ▲
                            templates/<template>.typ + data/<name>.yaml
```

The Go service shells out to the `typst` binary, which compiles the requested
template against the requested CV data file and streams the PDF back. A request
to `GET /cv/helsinki/cv-example.pdf` renders `data/cv-example.yaml` with
`templates/helsinki.typ`. Both templates use the **Lato** font shipped in
`fonts/` (passed to typst via `--font-path`).

Two templates ship, both reading the **same** `data/<name>.yaml`:

- **`helsinki`** — a two-column layout: a white main column with a full-height
  dark sidebar (skill/language rating bars, initials avatar).
- **`primeats`** — a single-column, **ATS-friendly** layout for automated resume
  parsers: no sidebar or rating bars, plain text, standard section headings,
  right-aligned dates, and an optional header photo.

To add your own template, drop a `templates/<name>.typ` file in and it's
reachable at `/cv/<name>/…` — start it with `#import "_common.typ": *` to reuse
the CV data and computed variables. (Shared partials are prefixed with `_` and
are not selectable as templates.)

## Your CV

Add a CV as **`data/<name>.yaml`** (the sample is `data/cv-example.yaml`, served
at `GET /cv/helsinki/cv-example.pdf`). Only `name` is required; every section is optional
and omitted from the PDF if absent. The schema (see the sample file for a full
example):

- **Header:** `name`, `title`, optional `photo` — a **root-absolute** path under
  `ROOT` (e.g. `/data/photo.svg`; `.svg`/`.jpg`/`.png` all work). If omitted,
  `helsinki` draws an initials avatar and `primeats` shows no photo.
- **Sidebar:** `details` (location, phone, email, nationality, driving_license,
  birth_date, birth_place), `links` (`label` + `url`), `skills` (`name` +
  `level` 1–10, drives a rating bar), `languages` (`name` + `level` as a
  free-text proficiency label, e.g. `Native / C2`, `B2`), and `hobbies`.
- **Main:** `summary`; `technical_skills` (a list of `{ category, items: [...] }`
  rendered as a bold category + comma-joined keywords); `experience` entries
  (`position`, `company`, `location`, `start`, `end`, and a free-form
  **`description`** written in **Markdown**); `projects` (`name`, optional
  `tech`, `description`, optional `url`); and `education` entries (`degree`,
  `institution`, `location`, `start`, `end`, optional Markdown `description`);
  and `certifications` entries (`name`, `issuer`, `date`, optional `url`); and
  `key_achievements` (a bulleted list where each item is a plain string or a
  `{ title, description }` pair rendered as **title** — description).
  Education/certifications do **not** count toward `{{experience_years}}`.

  The experience/education **`description`** is CommonMark Markdown — paragraphs,
  `**bold**`, `*italic*`, `` `code` ``, and (nested) bullet lists all render.
  It replaces the older `summary`/`highlights`/`subhighlights` fields. `{{var}}`
  placeholders are substituted before the Markdown is rendered.

### Computed variables

Reference values in the title, summary, and experience text via `{{name}}`
placeholders. Two are provided **automatically**:

- **`{{experience_years}}`** — total real work experience from your
  `experience` entries with month precision (`Present` = today). Overlapping
  jobs are **merged (counted once)** and gaps between jobs are **excluded**, so
  it's the union of the periods you actually worked. It renders as whole years
  with a trailing `+` when the leftover months are **≥ 6** (e.g. 10y6m → `10+`,
  10y5m → `10`). Education is not counted. Write it without a literal plus:
  `{{experience_years}} yrs`.
- **`{{age}}`** — whole-years age from the **global `birth_date`** field (which
  also feeds the sidebar). Dates may be `DD.MM.YYYY` or `YYYY-MM-DD`.

You can also define your own under `vars:`, each being a **literal**,
`{ since_year: N }` → *(current year − N)*, or `{ age_from: <date> }` →
whole-years age:

```yaml
birth_date: 15.04.1990      # global; drives {{age}} and the sidebar

vars:
  started_coding: { since_year: 2012 }        # → (this year - 2012)
  tagline:        "Open to remote roles"

summary: >
  Senior Java Developer ({{experience_years}}+ yrs), age {{age}} — building ...
```

Everything is computed at render time from the system date and your work
history, so the PDF is always current. Defining a var named `experience_years`
or `age` overrides the automatic one.

## API

| Method & path                  | Description                                                                 |
| ------------------------------ | --------------------------------------------------------------------------- |
| `GET /cv/<template>/<name>.pdf` | Renders `data/<name>.yaml` with `templates/<template>.typ`. `?download` returns it as a file download instead of displaying it in the browser. |
| `GET /healthz`                 | Liveness probe. Returns `{"status":"ok"}`.                                   |

Both `<template>` and `<name>` must be plain slugs (`[A-Za-z0-9_-]`); anything
else is rejected (400), and an unknown template or CV returns 404.

By default the PDF is served `inline`, so the URL is linkable — a browser opens
it in its viewer. Add `?download` (or `?download=1`) to get
`Content-Disposition: attachment` and a save dialog; `?download=0` forces inline
back on. The suggested filename is the CV name from the URL, e.g.
`cv-example.pdf`.

The service is read-only: it renders CVs that exist on disk under `DATA_DIR`,
and there is no upload endpoint. To add a CV, drop a `<name>.yaml` into `data/`
(it is a bind mount in `docker-compose.yml`, so no rebuild is needed) and
request `/cv/<template>/<name>.pdf`.

```bash
# Render a CV with a template:
curl -L http://localhost:8080/cv/helsinki/cv-example.pdf -o cv.pdf
curl -L http://localhost:8080/cv/primeats/cv-example.pdf -o cv-ats.pdf

# Force a browser save dialog rather than the inline PDF viewer:
curl -L "http://localhost:8080/cv/helsinki/cv-example.pdf?download" -o cv.pdf
```

## Caching

Rendered PDFs are cached in memory for `CACHE_TTL` (default 24h) and clients get
a matching `Cache-Control` plus an `ETag`, so a repeat request is answered
without touching typst and a revalidation costs one empty 304.

The cache key covers the template file, the CV file (path, size, mtime) and the
render date, so **editing a CV or a template takes effect on the next request** —
no restart, no manual purge. Cached responses and 304s are served before a
render slot is taken, so repeat traffic is never shed by
`MAX_CONCURRENT_RENDERS`.

Caching is `private` by default because a CV holds personal data; setting
`ALLOW_INDEXING=true` (a deliberately public portfolio) switches it to `public`
so a CDN may store it too. `CACHE_TTL=0` disables caching and sends `no-store`.

`{{age}}`, `{{experience_years}}` and `end: Present` resolve from the render
date, so a cached PDF goes stale the moment the date changes. The advertised
`max-age` is therefore **capped at local midnight**: with the default 24h TTL a
response served at 12:00 gets `max-age=43200`, and one served at 23:59 gets
`max-age=60`. Clients revalidate right after the rollover and pick up the new
values; the server-side key includes the date, so it re-renders on the first
request of the new day.

Renders are **serialised by default** (`MAX_CONCURRENT_RENDERS=1`) — with the
cache absorbing repeat traffic, only cold keys reach typst at all, so one
render at a time bounds CPU tightly. Raise it if you serve many distinct CVs
whose cold renders genuinely overlap.

## Exposure & privacy

The service has no authentication: anyone who can reach it can fetch any CV
whose URL they can guess, and a CV carries name, phone, email, city, date of
birth and birth place. `<name>` comes straight from the filename, so
`data/john-smith.yaml` is served at a guessable `/cv/<template>/john-smith.pdf`.

If the service is reachable from the internet, give each CV an unguessable
filename — the slug rules already allow it, so this needs no code change:

```bash
mv data/john-smith.yaml data/john-smith-7f3a9c2e.yaml
# -> /cv/helsinki/john-smith-7f3a9c2e.pdf
```

That turns the URL into a capability: still readable for whoever you send it
to, but not enumerable. PDFs are also served with `X-Robots-Tag: noindex,
nofollow` by default (see `ALLOW_INDEXING`), because an indexed CV cannot be
retroactively unpublished. Render failures return a generic 500 — the typst
error, which quotes template source and absolute paths, goes only to the log.

## Running

```bash
docker compose up --build     # http://localhost:8080
```

Locally (needs `typst` on PATH):

```bash
make run
```

## CI/CD

CI runs on every branch push and PR (`.github/workflows/ci.yml`): vet, tests
with `typst` installed, a cold-cache render that proves the vendored Typst
packages resolve offline, plus an image build and a container smoke test that
fetches a PDF from both templates.

Merging to `main` runs `.github/workflows/publish.yml`, which publishes to the
GitHub Container Registry:

```
ghcr.io/melancholic/cvrenderer:sha-<short>   # immutable, one per merge
ghcr.io/melancholic/cvrenderer:latest
```

The package is private, and both workflows run on the built-in `GITHUB_TOKEN`
— there are no secrets to configure. Pulling it elsewhere needs a token with
`read:packages`:

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u <username> --password-stdin
docker pull ghcr.io/melancholic/cvrenderer:latest
```

**Deployment is not automated** — the pipeline stops at the registry.

## Configuration (environment variables)

| Variable         | Default            | Description                                        |
| ---------------- | ------------------ | -------------------------------------------------- |
| `PORT`           | `8080`             | Listen port                                        |
| `ROOT`           | `/app`             | Directory typst may read from (`--root`)           |
| `TEMPLATE_DIR`   | `templates`        | Directory of `.typ` templates, relative to `ROOT`. `<template>` → `$TEMPLATE_DIR/<template>.typ` |
| `DATA_DIR`       | `data`             | Directory of CV `.yaml` files, relative to `ROOT`. `<name>` → `$DATA_DIR/<name>.yaml` |
| `FONT_DIR`       | `fonts`            | Extra font dir passed to typst (`--font-path`)     |
| `PACKAGE_CACHE_DIR` | `typst-packages` | Vendored Typst package cache (`--package-cache-path`), so `@preview/…` imports resolve offline |
| `TYPST_BIN`      | `typst`            | Path to the typst executable                       |
| `RENDER_TIMEOUT` | `30s`              | Per-render timeout                                 |
| `MAX_CONCURRENT_RENDERS` | `1`        | Max simultaneous `typst` processes; past this, requests queue then get 503 |
| `RENDER_QUEUE_TIMEOUT` | `10s`        | How long a request waits for a render slot before 503 |
| `CACHE_TTL`      | `24h`              | How long a rendered PDF is reused, and the client freshness window — capped at local midnight. `0` disables caching |
| `CACHE_MAX_ENTRIES` | `64`            | Cap on cached PDFs held in memory                  |
| `ALLOW_INDEXING` | `false`            | When false, PDFs are served with `X-Robots-Tag: noindex, nofollow` |

## Development

```bash
make test                                            # all tests
make test-one PKG=./internal/typst RUN=TestRenderMissingData   # a single test
make render                                           # render to bin/cv.pdf via typst
```

Render tests **skip** automatically when `typst` isn't installed, so `go test`
still passes in a bare environment.
