FROM node:22-alpine AS deps
WORKDIR /repo
RUN corepack enable
COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY apps/web/package.json apps/web/
COPY packages/api-client/package.json packages/api-client/
COPY packages/ui/package.json packages/ui/
RUN pnpm install --frozen-lockfile

FROM deps AS build
# NEXT_PUBLIC_* values are inlined into the client bundle at build time, so
# multi-user (Clerk) mode must be selected when the image is built. Empty
# defaults keep the default single-user token mode.
ARG NEXT_PUBLIC_AUTH_MODE=
ARG NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY=
ENV NEXT_PUBLIC_AUTH_MODE=$NEXT_PUBLIC_AUTH_MODE
ENV NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY=$NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY
COPY openapi.yaml ./
COPY packages ./packages
COPY apps/web ./apps/web
RUN pnpm --filter web build

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
# Standalone output (outputFileTracingRoot = repo root) mirrors the monorepo
# layout: server.js lives under apps/web, node_modules at the root.
COPY --from=build --chown=node:node /repo/apps/web/.next/standalone ./
COPY --from=build --chown=node:node /repo/apps/web/.next/static ./apps/web/.next/static
USER node
EXPOSE 3000
ENV PORT=3000
ENV HOSTNAME=0.0.0.0
CMD ["node", "apps/web/server.js"]
