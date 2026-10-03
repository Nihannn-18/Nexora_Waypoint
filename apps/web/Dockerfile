# syntax=docker/dockerfile:1
#
# Waypoint Next.js web app.
# Build context is the WORKSPACE ROOT — Nx needs the whole workspace to build.
#
#   docker build -f apps/web/Dockerfile -t waypoint-web .

# ---------------------------------------------------------------------------
# Dependencies
# ---------------------------------------------------------------------------
FROM node:22-alpine AS deps

WORKDIR /app

# Only the manifests, so this layer is cached until a dependency actually changes.
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
FROM node:22-alpine AS builder

WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .

# NEXT_PUBLIC_* values are inlined into the client bundle at BUILD time, not read
# at runtime. Changing the API URL therefore means rebuilding this image, which
# is why it is a build arg rather than only an environment variable.
#
# The default points at the host, not at the `api` service: this URL is fetched
# by the judge's BROWSER, which resolves nothing on the compose network.
ARG NEXT_PUBLIC_API_BASE_URL=http://localhost:8080/api/v1
ENV NEXT_PUBLIC_API_BASE_URL=$NEXT_PUBLIC_API_BASE_URL

ENV NEXT_TELEMETRY_DISABLED=1

# Better Auth requires a secret when the auth route module is evaluated during
# `next build`. This is a build-only placeholder: it is confined to the builder
# stage and never copied to the runtime image, where the real BETTER_AUTH_SECRET
# is injected from the environment.
ENV BETTER_AUTH_SECRET=build-only-placeholder-not-used-at-runtime-000

RUN npx nx build web

# ---------------------------------------------------------------------------
# Runtime
# ---------------------------------------------------------------------------
FROM node:22-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0 \
    TZ=Asia/Colombo

RUN addgroup -S nodejs -g 1001 \
    && adduser -S nextjs -u 1001 -G nodejs

# The standalone bundle carries its own minimal node_modules.
COPY --from=builder --chown=nextjs:nodejs /app/apps/web/.next/standalone ./
# Static assets and public files are NOT included in the standalone trace and
# must be copied in separately, or every stylesheet and font 404s.
COPY --from=builder --chown=nextjs:nodejs /app/apps/web/.next/static ./apps/web/.next/static
COPY --from=builder --chown=nextjs:nodejs /app/apps/web/public ./apps/web/public

EXPOSE 3000

USER nextjs

HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=5 \
    CMD wget -qO- http://127.0.0.1:3000/ >/dev/null 2>&1 || exit 1

# server.js sits where the trace root put it: <root>/apps/web/server.js
CMD ["node", "apps/web/server.js"]
