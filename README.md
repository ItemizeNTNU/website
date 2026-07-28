# itemize.no

The website for [Itemize NTNU](https://itemize.no) — a student organisation for
information security and CTFs at NTNU Trondheim.

A single Go binary. Templates, stylesheets, scripts, fonts, images and the
editable content are all compiled in via `embed.FS`, so there is nothing to
build and nothing to deploy alongside it.

## Requirements

- Go 1.26+
- MongoDB (events and check-in attendance)
- A FusionAuth tenant (members, roles, login)
- Optionally a Discord application + bot (event sync, account linking)

## Running locally

```sh
cp .env.example .env      # then fill it in
docker compose up -d mongo
go run ./cmd/website -dev
```

`-dev` reads templates, CSS and content from disk instead of the embedded
copies, so edits show up on reload without recompiling.

Discord credentials may be left empty: event sync is skipped and the
account-linking routes return 503, but everything else works.

## Editing content

Board members, the resource directory, contact details and social links live in
[`content/`](content/) as YAML — no Go knowledge needed. The files are validated
at startup, so a malformed URL or a bad icon name fails the deploy loudly rather
than rendering a blank page.

Updating the board after a general assembly is a one-file edit to
[`content/styret.yaml`](content/styret.yaml).

## Layout

| Path | What lives there |
|---|---|
| `cmd/website/` | entry point — config, wiring, graceful shutdown |
| `assets/` | templates, CSS, JS, fonts, images (embed root) |
| `content/` | editable YAML content (embed root) |
| `internal/config/` | environment loading and startup validation |
| `internal/httpx/` | middleware chain, gzip, static serving |
| `internal/auth/` | OIDC, sessions, role gating, CSRF |
| `internal/events/` | event model, MongoDB repository, Discord sync |
| `internal/users/` | registration and profile |
| `internal/fusionauth/`, `internal/discord/` | external API clients |
| `internal/web/`, `internal/api/` | HTML and JSON handlers |
| `old-website/` | the previous Sapper app, kept for reference only |

`assets/` and `content/` sit outside `internal/` because `//go:embed` cannot
reference parent directories — and keeping them at the top level means a board
member editing content never has to open a Go file.

## Testing

```sh
go test ./...
```

Tests that need a database are skipped unless `MONGO_TEST_URL` is set:

```sh
MONGO_TEST_URL=mongodb://localhost:27017/website_test go test ./...
```

## Deployment

Pushes to `main` build and publish `ghcr.io/ItemizeNTNU/website:latest` and then
trigger a redeploy webhook; `dev` publishes the `dev` tag. Every build is also
tagged `sha-<short>`, so a rollback is a `docker pull` of the previous SHA.

## History

Before 2026 this site ran on Sapper (the predecessor to SvelteKit) with an
Express server. That stack was deprecated in 2020 and its dependency tree could
no longer be kept current by a board that turns over every year. The rewrite
trades framework features for longevity: standard library only where it counts,
a handful of dependencies, and no build step to rot.

The old application is preserved under [`old-website/`](old-website/) for
reference. It is not built, deployed, or maintained.
