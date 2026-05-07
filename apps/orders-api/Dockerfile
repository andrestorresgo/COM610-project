# =============================================================================
# Stage 1: deps — production dependencies only (shared by dev + prod)
# =============================================================================
FROM oven/bun:alpine AS deps

WORKDIR /app

COPY package.json bun.lock ./

RUN bun install --frozen-lockfile --production


# =============================================================================
# Stage 2: dev — hot-reload development target
#
# Source code is NOT copied here. docker-compose mounts the host directory at
# /app at runtime. node_modules are protected by a named volume so the mount
# does not shadow the installed packages (see docker-compose.yml).
# =============================================================================
FROM oven/bun:alpine AS dev

WORKDIR /app

COPY package.json bun.lock ./

# Install ALL deps (including devDependencies) for the dev environment.
RUN bun install --frozen-lockfile

# Bun's --hot flag does in-process hot reloading without restarting the
# process. If you prefer a full restart on change, use --watch instead.
CMD ["bun", "run", "--hot", "src/index.ts"]


# =============================================================================
# Stage 3: builder — compile & bundle for production
# =============================================================================
FROM oven/bun:alpine AS builder

WORKDIR /app

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY . .

# --target=bun   → emits Bun-native code (no Node.js compat overhead)
# --minify       → minifies identifiers, whitespace, and syntax
# --sourcemap=none → strip sourcemaps from the prod bundle
RUN bun build src/index.ts \
      --target=bun \
      --outfile=dist/server.js \
      --minify \
      --sourcemap=none


# =============================================================================
# Stage 4: prod — minimal distroless runtime image
# =============================================================================
FROM oven/bun:distroless AS prod

WORKDIR /app

# Copy only the compiled bundle and production node_modules.
# This keeps the final image lean: no source, no devDeps, no build tools.
COPY --from=deps    /app/node_modules ./node_modules
COPY --from=builder /app/dist/server.js ./server.js

ARG PORT=3000
ENV PORT=${PORT}
ENV NODE_ENV=production

EXPOSE ${PORT}

# distroless has no curl/wget. Use Bun's built-in fetch for the health probe.
# Your Hono app MUST expose GET /health → 200 OK.
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
  CMD ["bun", "--eval", "const r = await fetch(`http://localhost:${process.env.PORT}/health`); if (!r.ok) process.exit(1);"]

ENTRYPOINT ["bun", "server.js"]
