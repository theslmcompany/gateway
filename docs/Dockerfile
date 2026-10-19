# ── Build stage ──────────────────────────────────────────────────────────────
FROM node:26-alpine AS builder

WORKDIR /app
# Pin pnpm to the version that wrote pnpm-lock.yaml. Bare `corepack enable`
# resolves to whatever is latest (pnpm 11 today), which re-reconciles the
# pnpm-10 lockfile mid-build and fails.
RUN corepack enable && corepack prepare pnpm@10.17.1 --activate

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY . .
RUN pnpm build

# ── Serve stage ──────────────────────────────────────────────────────────────
FROM nginx:1.31-alpine

COPY --from=builder /app/.vitepress/dist /usr/share/nginx/html

# Healthcheck endpoint
RUN echo "OK" > /usr/share/nginx/html/health

COPY nginx.docs.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
