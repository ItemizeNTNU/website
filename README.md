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
| `docs/` | operational notes: authentication, FusionAuth setup |

`assets/` and `content/` sit outside `internal/` because `//go:embed` cannot
reference parent directories — and keeping them at the top level means a board
member editing content never has to open a Go file.

## Authentication

Login runs against FusionAuth over OpenID Connect (authorization code + PKCE);
the session is a sealed cookie with no server-side store. See
[`docs/auth.md`](docs/auth.md).

> **Open item — ID tokens are verified with HS256.**
>
> HS256 signs with a shared secret, which means anyone holding it can *mint*
> tokens rather than only verify them. RS256 would leave the signing key with
> FusionAuth alone. The switch has not been made yet because the FusionAuth
> tenant is shared with other applications and it needs checking whether the
> signing key is set per application or tenant-wide.
>
> Both code paths exist and are tested; `FUSION_AUTH_ID_TOKEN_ALG` selects one.
> [`docs/auth.md`](docs/auth.md) has the migration steps and what to delete
> afterwards. The relevant places carry `TODO(auth)` comments.

FusionAuth admin steps live in
[`docs/fusionauth-checklist.md`](docs/fusionauth-checklist.md).

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
tagged `sha-<short>`, so a rollback is a `docker pull` of a known digest rather
than archaeology through the registry.

The image is `distroless/static` — no shell, no package manager — and about
18 MB. Since everything is embedded there is nothing to mount alongside it. The
container health check is the binary probing itself (`/website -healthcheck`),
because there is no curl to call.

`ENV` governs strictness, not asset loading:

- `ENV=production` requires `BASE_URL` and `FUSION_AUTH_HOST` to be https, logs
  as JSON, and ignores any `.env` file in the image.
- Anything else relaxes the https requirement and logs as text — useful on a
  staging host without a certificate.

Assets are always served from inside the binary. Only the `-dev` flag reads
them from the working directory, and that is for running from a checkout.

A failed start leaves the previous container serving, so the server refuses to
come up rather than run degraded: MongoDB must answer, and OIDC discovery must
succeed (retried five times with backoff, since the identity provider and the
site often start together).

## What runs where

| | |
|---|---|
| Members, roles, passwords | FusionAuth |
| Events and attendance | MongoDB |
| Announcements, member role | Discord |
| Everything else | this binary |

Discord and the FusionAuth API key are both optional. Without them the site
still runs and still logs people in; event announcements are skipped, and
registration and account linking say they are unavailable rather than failing
on submit.

## History

Before 2026 this site ran on Sapper (the predecessor to SvelteKit) with an
Express server. That stack was deprecated in 2020 and its dependency tree could
no longer be kept current by a board that turns over every year. The rewrite
trades framework features for longevity: standard library only where it counts,
a handful of dependencies, and no build step to rot.

The old application was kept under `old-website/` for reference until August
2026 and remains available in git history. It is not built, deployed, or
maintained.
