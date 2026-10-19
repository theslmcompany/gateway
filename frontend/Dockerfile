# ── Build stage ─────────────────────────────────────────────────────────────
FROM node:26-alpine AS builder

RUN corepack enable && corepack prepare pnpm@10.17.1 --activate

WORKDIR /app

# pnpm-workspace.yaml carries the security `overrides` and `auditConfig`; without
# it `--frozen-lockfile` fails, because the lockfile records overrides the
# in-container config would not have.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY . .

ARG VITE_API_URL=""
RUN pnpm build

# ── Runtime stage ────────────────────────────────────────────────────────────
FROM nginx:1.31-alpine

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
