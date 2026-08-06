# syntax=docker/dockerfile:1

# ── Build ──────────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS build

WORKDIR /src

# Dependencies first, so a source-only change does not re-download them.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

ARG VERSION=dev
# CGO_ENABLED=0 gives a static binary and selects Go's own DNS resolver, which
# is what makes the distroless stage below work: it has no libc and no
# /etc/nsswitch.conf for the cgo resolver to read.
#
# -trimpath keeps the build reproducible; the ldflags strip the symbol table
# and stamp the version reported by /healthz.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/website ./cmd/website

# ── Run ────────────────────────────────────────────────────────────────────
# distroless/static: no shell, no package manager, nothing to exploit that we
# did not put there. It carries CA certificates, which are needed for TLS to
# FusionAuth and Discord.
#
# Templates, stylesheets, fonts, images and content are all embedded in the
# binary, so there is genuinely nothing else to copy.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/website /website

USER nonroot:nonroot
EXPOSE 3000

# There is no shell and no curl, so the health check is the binary probing
# itself. Without this an orchestrator has to know to probe /healthz over HTTP;
# with it, `docker compose` alone reports the container's health.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD ["/website", "-healthcheck"]

ENTRYPOINT ["/website"]
