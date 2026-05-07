# =============================================================================
# Stage 1: base — Go SDK with dependencies downloaded
#
# Used as the foundation for both dev and prod branches. Keeping dependency
# download here means Docker caches this layer until go.mod/go.sum change.
# =============================================================================
FROM golang:alpine AS base

# ca-certificates: needed if the app makes outbound HTTPS calls.
# tzdata: needed if the app uses non-UTC time zones.
# git: needed by some go mod download calls that resolve VCS paths.
RUN apk add --no-cache ca-certificates tzdata git

WORKDIR /app

# Copy dependency manifests first for layer caching.
# IMPORTANT: go.sum must be included or `go mod download` will fail.
COPY go.mod go.sum ./
RUN go mod download && go mod verify


# =============================================================================
# Stage 2: dev — hot-reload development target
#
# Source code is NOT copied here. docker-compose mounts the host directory at
# /app at runtime, so Air sees live edits immediately.
#
# Air is installed only in this branch; the prod builder never sees it.
# =============================================================================
FROM base AS dev

# Install Air for live-reloading. Pin a specific version in production
# workflows; @latest is acceptable for a dev tool.
RUN go install github.com/air-verse/air@latest

# Air reads its config from .air.toml in the working directory.
# Ensure your repo has a valid .air.toml (or pass -c to the CMD below).
CMD ["air"]


# =============================================================================
# Stage 3: builder — compile a static binary for production
# =============================================================================
FROM base AS builder

COPY . .

# Build flags:
#   CGO_ENABLED=0   → pure-Go binary, no libc dependency → works in distroless
#   GOOS=linux      → explicit cross-compile target
#   -ldflags "-s -w" → strip debug info & DWARF tables (~30% smaller binary)
#   -trimpath       → remove local filesystem paths from binary
#   -tags netgo     → use Go's pure-Go DNS resolver
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w" \
      -trimpath \
      -tags netgo \
      -o /app/server \
      ./cmd/server
# Adjust ./cmd/server to match your actual main package path.


# =============================================================================
# Stage 4: prod — minimal distroless runtime image
#
# gcr.io/distroless/static-debian12:nonroot ships with ca-certificates, tzdata,
# and a nonroot user (uid 65532) already baked in. ~2MB larger than scratch
# but it supports HEALTHCHECK CMD and avoids manual certificate copies.
# =============================================================================
FROM gcr.io/distroless/static-debian12:nonroot AS prod

COPY --from=builder /app/server /server

ARG PORT=8080
ENV PORT=${PORT}
ENV APP_ENV=production

EXPOSE ${PORT}

# This requires your Go binary to handle a -healthcheck flag:
#   flag.Bool("healthcheck", false, "run health probe and exit")
# The flag should GET http://localhost:$PORT/health and exit 0/1 accordingly.
# Alternative if you don't want to modify the binary: switch to
#   gcr.io/distroless/base-debian12 which includes wget.
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/server", "-healthcheck"]

ENTRYPOINT ["/server"]
