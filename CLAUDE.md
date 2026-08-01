# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A self-hosted HTTP service that renders a CV to PDF from a YAML file, using **Typst** as the engine. The Go service is a thin orchestrator: it shells out to the `typst` CLI and streams the resulting PDF. No database, no auth, no external API. Stateless request → PDF. The Go module is `github.com/Melancholic/cvrenderer`.

## Commands

Go is not on PATH in this environment; a toolchain lives at `/home/sosnov/.local/go/bin` and the `typst` binary at `/home/sosnov/.local/bin`. Export both and set `GOPATH=/home/sosnov/.local/gopath` before running `go`/`make`.

```bash
make run        # run locally (needs typst on PATH)
make build      # static binary -> bin/cvrenderer
make test       # all tests (render tests skip if typst absent)
make render     # render data/cv-example.yaml -> bin/cv.pdf via typst directly
make vet
make up         # docker compose up --build (serves :8080)

# single test:
make test-one PKG=./internal/typst RUN=TestRenderMissingData
```

Docker is not installed in this environment, so the image/compose path can only be exercised on the deploy host.

## Architecture

```
HTTP handler ──▶ typst.Renderer ──▶ `typst compile` ──▶ application/pdf
(internal/server)  (internal/typst)      (subprocess)
                        ▲
   templates/<template>.typ + data/<name>.yaml   (both chosen per request)
```

- **`internal/typst`** — the only code that runs the `typst` binary. `Render(templatePath, dataPath)` compiles a template against a data file and streams the PDF from typst's stdout (nothing hits disk on output) — it is the package's whole surface. **Neither the template nor the data file is fixed on the `Renderer` struct — both are passed per call**, because the HTTP layer selects them per request. There is no render-from-bytes path: the service only ever renders files that already exist under `DATA_DIR`, so nothing is written to disk on the input side either.

- **`internal/server`** — routing (Go 1.22 method-pattern `ServeMux`), the handlers, PDF/JSON response shaping. Three routes: `GET /cv/{name}` renders `data/<name>.yaml` (the `.pdf` suffix is stripped off `{name}` in the handler — a URL without it is a 404), `GET /cv.pdf` does the same for `DEFAULT_CV`, and `GET /healthz`. Both CV routes funnel into `serveCV`. **`resolveCV` tries `.yaml` then `.yml`** (`dataExts`), so both spellings live at the same extensionless URL and `.yaml` wins when a name exists as both; the chosen extension goes into `resolved.path` and therefore into the cache key, so a rename invalidates instead of replaying stale bytes. Two query parameters: **`?template=`** picks the layout (absent/empty → `DEFAULT_TEMPLATE`; **the template is not a path segment** — it moved out of the path deliberately, since only the owner's CVs are served and the layout is a view option, not an identity), and **`?download`**, which flips `Content-Disposition` from `inline` (the default, so `/cv/…` links open in the browser viewer) to `attachment`; `?download=0`/`=false` opts back out, any other value counts as true. **The service is read-only — there is no upload endpoint** (a `POST /render` taking a YAML body was removed deliberately; don't re-add one without asking, it was the only thing that ever wrote request data to disk). **The CV name and the `template` value are slug-validated** (`^[A-Za-z0-9][A-Za-z0-9_-]*$`, no dots/slashes) and the resolved file must exist, so requests can only reach our own `data/*.yaml` and `templates/*.typ` — this is the path-traversal guard, don't weaken it. Invalid slug → 400, unknown file → 404.

- **`internal/server/urls.go`** — the startup URL listing. `Server.URLs()` walks `DATA_DIR` × `TEMPLATE_DIR` (applying the same slug guard, so `_common` and non-slug filenames never appear) and returns every renderable URL under `VIRTUAL_HOST`; the default template is represented by the bare URL rather than a spelled-out `?template=`, and `DEFAULT_CV` also gets its `/cv.pdf` lines — first, since that's the URL to hand out. `LogURLs()` prints them from `main` at startup and warns when `DEFAULT_CV`/`DEFAULT_TEMPLATE` name a file that doesn't exist (otherwise a typo'd default surfaces only as a browser 404). Technical routes (`/healthz`) are deliberately omitted. Keep this in step with the routes — it is the only index the service has.

- **`internal/config`** — env-var loading with defaults; must run with zero config in-container. Note `MAX_CONCURRENT_RENDERS` defaults to 1 (serialised renders), which the cache makes affordable. Dirs: `DATA_DIR` (default `data`) and `TEMPLATE_DIR` (`templates`). `DEFAULT_CV` (`cv-default`) is the CV behind `/cv.pdf` and `DEFAULT_TEMPLATE` (`helsinki`) the layout for a request with no `?template=`; **the repo ships no `data/cv-default.yml`** (the sample is `cv-example.yaml`), so `/cv.pdf` 404s until the owner adds their own — that's intentional, the startup log warns about it. `VIRTUAL_HOST` (`localhost:8080`) affects nothing but the startup URL log, and `OUT_FILE_PREFIX` (`resume-`) only the suggested download filename.

- **`templates/*.typ`** — the Typst templates, selected by filename: `?template=helsinki` → `templates/helsinki.typ`. Two ship today: **`helsinki.typ`** (two-column, white main + full-height navy sidebar) and **`primeats.typ`** (single-column, ATS/parser-friendly — no sidebar/bars, blue section rules, right-aligned dates, optional header photo). Both `#import "_common.typ"` for the shared data + computed-vars logic and only define layout. **`data/cv-example.yaml`** — the sample CV and schema example, served e.g. at `GET /cv/cv-example.pdf` or `GET /cv/cv-example.pdf?template=primeats`. **`fonts/`** — the Lato font files shipped so rendering is identical everywhere.

- **Shared partials use a `_` prefix (`templates/_common.typ`).** Anything in `templates/*.typ` is otherwise selectable as a template, so shared modules must be hidden from the router. The `_` prefix does that *for free* via the slug guard: `?template=_common` fails validation (slugs must start `[A-Za-z0-9]`) → 400, and `?template=common` finds no file → 404. New shared/partial `.typ` files MUST start with `_`; real templates must not.

## Non-obvious things that will bite you

- **The render cache is keyed on the date, and that is load-bearing.** `internal/server/cache.go` memoises rendered PDFs for `CACHE_TTL` (default 24h). The key is sha256 over the template and data files (path + size + mtime) **plus the render date** — the date must stay in the key because templates resolve `{{age}}`, `{{experience_years}}` and `end: Present` from `datetime.today()`, so identical files legitimately produce different bytes tomorrow. Size+mtime means editing a CV or template invalidates on the next request with no restart. The `ETag` is derived from the *inputs*, not from hashing the PDF, which is only sound because typst output is byte-deterministic for identical inputs (verified: two runs of the same input hash identically) — if that ever stops holding, the ETag strategy has to change. Cache hits and 304s are answered **before** `acquireRenderSlot`, so repeat traffic is never shed by the concurrency cap; keep that ordering. `s.now` exists so tests can cross a day boundary without waiting. For the same reason the advertised `max-age` is `cacheMaxAge()` = **`CACHE_TTL` capped at local midnight**, not the raw TTL — otherwise a client fetching at 23:00 would hold date-derived values for hours after they changed. Local time is correct here because typst's `datetime.today()` runs in the same process's timezone.

- **The endpoint is unauthenticated, so failure responses must stay opaque and cheap.** Three coupled rules in `handleCV`: (1) a render failure returns a generic `"failed to render CV"` with the typst error going to the **log only** — typst's stderr quotes template source lines and absolute paths. It is deliberately **500, not 404**: the file exists and is readable, so masking a broken template as "not found" would hide real faults from monitoring, and it leaks strictly less than the 200 a working CV already returns to the same guesser. (2) Every render is capped by the `renderSlots` semaphore (`MAX_CONCURRENT_RENDERS`, **default 1 — renders are serialised**) — a typst process is CPU-bound, so an uncapped public endpoint is a load-amplification target, and the render cache means only cold keys reach typst at all; over capacity requests queue for `RENDER_QUEUE_TIMEOUT` then get 503 + `Retry-After`. (3) PDFs carry `X-Robots-Tag: noindex, nofollow` unless `ALLOW_INDEXING=true`. Since `<name>` is the real filename, a guessable filename means a guessable CV — the README recommends an unguessable suffix (capability URL) for public deployments, and note `DEFAULT_CV` is reachable at `/cv.pdf` with no guessing at all.

- **The download filename is `OUT_FILE_PREFIX` + timestamp, not the CV name.** `outFilename()` returns `<OUT_FILE_PREFIX><YYYYMMDDHHMM>.pdf` (default prefix `resume-`, timestamp from `s.now()` so tests can pin it) — the data filename never reaches the client. It's a response header only: it is **not** in the cache key, so two fetches a minute apart legitimately replay identical bytes under different filenames, and the minute resolution means the same PDF is saved under a new name on each download.

- **Never build `Content-Disposition` by concatenation.** `writePDF` uses `mime.FormatMediaType`, which quotes/escapes the filename and percent-encodes non-ASCII into a `filename*` parameter. This is what makes `OUT_FILE_PREFIX` safe to interpolate: a prefix containing CRLF comes back percent-encoded (verified in `TestOutFilePrefix`) rather than splitting the header. The header value was previously interpolated by hand, which was only safe because the filename was a validated slug — with the prefix now coming from the environment, hand-built headers would be a CRLF-injection hole. `FormatMediaType` returns `""` on an unrepresentable value, so keep the fallback that sends the bare disposition.

- **Typst's sandbox + path resolution.** Typst only reads files under `--root`, and root-absolute paths (leading `/`) resolve *relative to `--root`*, not the OS root. The handler passes the CV data as `/<DATA_DIR>/<name>.yaml` (e.g. `/data/cv-example.yaml`), which with `ROOT=/app` reads `/app/data/cv-example.yaml`. That's why the container mounts CV data at `/app/data`, not `/data`. Any new file the template reads must live under `ROOT`.

- **Template path is CWD-relative, not root-relative.** `typst compile` resolves the *input file* argument against the process working directory, so `typst.Renderer` sets `cmd.Dir = Root`. Don't remove that — tests and prod both rely on it.

- **`photo` must be a root-absolute path.** Typst resolves `image()` paths *relative to the template file* (`templates/`), not the data file — so a bare `data/photo.svg` would look for `templates/data/photo.svg`. The `photo:` YAML field must therefore be root-absolute (e.g. `/data/photo.svg` → `$ROOT/data/photo.svg`), matching how the data file itself is addressed. `data/photo.svg` is a shipped placeholder (a generic silhouette, not a real face); both templates read the same `photo` field — `helsinki` clips it into its circular avatar, `primeats` shows it as a top-right rectangle.

- **Typst closures can't mutate outer variables** (`error: variables from outside the function are read-only`). In `helsinki.typ`, array-building (e.g. the contact line) is done in a `#{ ... }` code block, not a helper function. Use `field(dict, "key")` for optional access — indexing a missing key is a hard error in Typst.

- **Fonts.** Typst's embedded fonts are serif-only (Libertinus, New Computer Modern) plus DejaVu Sans/Mono — there is no bundled sans-serif. So **Lato is shipped in `fonts/`** and passed via `--font-path` (config `FONT_DIR`, default `fonts`, resolved relative to the typst working dir = Root); both `helsinki` and `primeats` use it. If a template uses a new face, add its `.ttf` to `fonts/`; don't assume a system font exists — the container has none, and host-only fonts (e.g. Ubuntu Mono, Arial) that `typst fonts` may list locally won't be present in the image. Verify container-safe rendering with `HOME=/tmp/empty-home typst fonts --font-path fonts`.

- **Markdown descriptions + vendored packages (offline).** Experience/education entries use a single Markdown **`description`** field (paragraphs + nested bullet lists), rendered via the `@preview/cmarker:0.1.1` Typst package through the `md-body()` helper (which runs `subst()` first, then `cmarker.render(...)` with the CV's list markers `•/○/‣`). This replaced the old `summary`/`highlights`/`subhighlights` fields. cmarker is **vendored** into `typst-packages/preview/cmarker/0.1.1/` (self-contained WASM plugin, no transitive deps) and passed via `--package-cache-path` (config `PACKAGE_CACHE_DIR`, default `typst-packages`, resolved relative to Root) so `@preview/…` imports resolve with **no network access** at render time. The import runs on *every* render, so any Root other than the repo root must be given an absolute `PackageCacheDir` pointing at the real vendored dir. To add another `@preview` package, copy its `~/.cache/typst/packages/preview/<name>/<version>/` tree into `typst-packages/preview/` and confirm offline resolution with `HOME=/tmp/empty-home typst compile --package-cache-path typst-packages …`.

- **Computed variables.** `templates/helsinki.typ` substitutes `{{name}}` placeholders in the title/summary/experience text (`subst()`). Two are auto-provided: `{{experience_years}}` = `experience-months()`, which builds each job's `[month-index(start), month-index(end)]` interval (`Present`→today), sorts, **merges overlapping/touching intervals**, and sums the union — so overlaps count once and gaps are excluded — rendered as whole years with a trailing `+` when leftover months `>= 6` (so the placeholder already carries the plus — the sample summary text has no literal `+`) — real work only, education excluded; and `{{age}}` from the **global top-level `birth_date`** (not `details.birth_date` — that field was hoisted so age and the sidebar share one source). User `vars:` forms: literal, `{ since_year: N }`, `{ age_from: <date> }`; a user var named `experience_years`/`age` overrides the auto one. `parse-date` accepts `DD.MM.YYYY` and `YYYY-MM-DD`; `month-index` parses `Mon YYYY`/`YYYY`/`Present`. All evaluated at render time from `datetime.today()` in Typst (not Go). Typst has no `return`; helpers use if/else expressions.

- **Full-height sidebar + multi-page.** The navy sidebar is painted by `set page(background: place(right, rect(height: 100%, fill: navy)))` so it repeats on every page. Content is a single-row two-column `grid`; the main cell breaks across pages (Typst grid cells are breakable), and on later pages the sidebar column is empty while the painted background persists. Verified with a 2+ page CV.

- **Render tests skip when `typst` is missing** (`exec.LookPath`), so `go test` is green in bare environments but only meaningfully covers rendering where typst exists. `TestRenderTemplateSelection` compiles *every* shipped template against `data/cv-example.yaml`, so a template that only breaks on some field fails the suite rather than a request — keep it in step with `templates/*.typ`.

## CI/CD

Two workflows, and **the pipeline stops at the registry** — nothing is deployed anywhere yet. `ci.yml` runs on every non-`main` push and on PRs; `publish.yml` runs on merge to `main` and pushes `ghcr.io/melancholic/cvrenderer:sha-<short>` + `:latest` to GHCR (private package). No secrets are configured: both workflows run on the built-in `GITHUB_TOKEN`.

- **CI must install typst or it tests nothing.** The render tests skip on `exec.LookPath` failure, so a runner without typst produces a green suite that never compiled a template. `ci.yml` installs it, reading the version out of the `ARG TYPST_VERSION` line in the Dockerfile so CI and the image can't drift. There's also a cold-cache render step (`HOME=$(mktemp -d)`) that fails if the vendored `@preview/cmarker` tree or `--package-cache-path` is wrong — a warm user cache would otherwise mask it.

- **Actions are pinned to full commit SHAs**, with dependabot bumping them (a tag like `@v3` can be repointed by whoever owns that repo). The repo is public, so **`pull_request_target` must never be added to a workflow holding secrets** — that trigger runs with secrets in the base-repo context and would hand them to any PR author. Fork PRs get no secrets under the current `pull_request` trigger.

- **The `org.opencontainers.image.source` label is load-bearing**, not decoration: it links the package to the repository, which is what lets a workflow's `GITHUB_TOKEN` read the image while the package stays private.

## Conventions

- `internal/typst` is the seam for the renderer; keep subprocess handling there rather than calling `exec` from handlers. It has no HTTP knowledge.
- The service holds no secrets and stores nothing — keep it that way; it renders CVs from a local YAML file and needs no credentials.
