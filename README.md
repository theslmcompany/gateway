# AI Gateway

> Secure, observable middleware for agentic AI systems — an OpenAI-compatible
> proxy with multi-tenancy, guardrails, LLM routing, and end-to-end tracing,
> plus `aigw`, a CLI that gives teams logins instead of shared API keys — every
> call attributed and capped per user.

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22%2B-00ADD8.svg)](https://go.dev)
[![Docs](https://img.shields.io/badge/docs-live-brightgreen.svg)](https://gateway.theslmcompany.com/docs/)

📖 **[Documentation](https://gateway.theslmcompany.com/docs/)** · [Quickstart](https://gateway.theslmcompany.com/docs/quickstart) · [CLI](https://gateway.theslmcompany.com/docs/cli)

AI Gateway sits between your applications and upstream LLM providers. It speaks
the OpenAI and Anthropic APIs, so existing clients work unchanged, while adding
tenant isolation, guardrail enforcement, routing with circuit breaking, and full
OpenTelemetry tracing on the hot path.

Two pieces ship together:

| Piece | What it does |
|---|---|
| **Gateway** (`backend/`) | The proxy + admin API. Every LLM call is authenticated, policy-checked, routed, and traced. |
| **`aigw` CLI** (`cli/`) | Governance at the developer's shell. Mints a gateway key and injects it into any SDK's env vars — no provider key ever lands on a laptop. |

---

## 60-Second Quickstart

Full walkthrough: [`docs/quickstart.md`](docs/quickstart.md).

**Prerequisites:** Docker + Docker Compose, `jq`, and an API key from an upstream
provider (OpenAI, Anthropic, …).

```bash
git clone https://github.com/theslmcompany/gateway.git
cd gateway/backend
cp .env.example .env
```

Edit `.env` — set at minimum:

```ini
ADMIN_PASSWORD=<pick-one>
JWT_SECRET=<random-32-bytes>
UPSTREAM_0_API_KEY=sk-<your-openai-key>
UPSTREAM_0_MODEL=gpt-4o
```

Start the stack:

```bash
make up          # gateway + postgres + redis + clickhouse + otel + jaeger + ML sidecar
```

> First run builds the ML sidecar image (torch + transformers, several GB) —
> budget 5–10 minutes. Later runs start in seconds. Set `MLRUNNER_ENABLED=false`
> in `.env` to run without managed detectors.

Mint a key and send a request:

```bash
export ADMIN_EMAIL=admin@example.com ADMIN_PASSWORD=<the-one-you-set>

TOKEN=$(curl -s localhost:8081/api/v1/auth/login -H 'Content-Type: application/json' \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" | jq -r .token)

TENANT=$(curl -s localhost:8081/api/v1/tenants -H "Authorization: Bearer $TOKEN" | jq -r '.[0].ID')

KEY=$(curl -s localhost:8081/api/v1/tenants/$TENANT/keys \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"quickstart"}' | jq -r .key)

curl localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}'
```

The trace lands in Jaeger at <http://localhost:16686>.

> **`model` must exactly match an upstream you registered.** The router matches
> model names literally and per tenant — `gpt-4o-mini` fails if you only
> configured `gpt-4o`. This is the most common first-request error.

---

## Point an Existing App at the Gateway

No code changes — only the base URL and key change.

```bash
export OPENAI_BASE_URL=http://localhost:8080/v1
export OPENAI_API_KEY=$KEY          # gateway key (sk_...), not the provider key
python app.py
```

Anthropic SDKs and Claude Code use the native passthrough surface:

```bash
export ANTHROPIC_BASE_URL=http://localhost:8080
export ANTHROPIC_API_KEY=$KEY
claude
```

Or let the CLI do it — see below.

---

## Governance CLI (`aigw`)

**Stop handing out the company API key.** Sharing one provider key across
developers, QA, and CI gives you a single bill with no idea who spent what, no
per-person cap, and a long-lived secret on every laptop. `aigw` replaces that
key with a login: a team member runs `aigw login`, and the CLI mints a
tenant-scoped gateway key on demand and injects it into whatever SDK env vars
their tool expects. The provider key never leaves the gateway — so every call is
attributed and capped per tenant/user, access is revoked per person, and no
secret ever lands on a machine.

It logs in via your IdP, mints the key, caches it in the OS keyring, and exports
it into the SDK env vars a tool already reads.

```bash
cd cli && make install                            # -> $GOBIN/aigw

aigw config set endpoint http://localhost:8081    # admin API
aigw config set proxy    http://localhost:8080    # proxy SDKs hit
aigw login                                        # browser SSO (OIDC)

eval "$(aigw env openai)"          # OPENAI_API_KEY + OPENAI_BASE_URL in this shell
aigw run anthropic -- claude       # scope the vars to one child process
aigw env generic --write .env      # upsert AIGW_* into a dotenv file
aigw env openai --refresh          # mint a fresh key, rotating the cached one
```

| Command | Purpose |
|---|---|
| `aigw login` / `whoami` / `logout` | Browser SSO session, stored in the OS keyring |
| `aigw tenants list` | Tenants you can see |
| `aigw keys list \| create \| revoke` | Tenant API key lifecycle (plaintext shown once) |
| `aigw env <provider>` | Print shell exports pointing an SDK at the gateway |
| `aigw run <provider> -- <cmd>` | Run a command with those vars set, and nothing else |
| `aigw token` | Print the session JWT for curl/scripts |

Providers: `openai`, `anthropic`, `generic`.

> `aigw login` is SSO-only — it needs at least one OIDC provider configured on
> the gateway (`OIDC_GOOGLE_*` or `OIDC_MICROSOFT_*`, see [`docs/sso.md`](docs/sso.md)).
> Without SSO, create keys through the admin UI or the admin API as in the
> quickstart above.

Details: [`docs/cli.md`](docs/cli.md).

---

## What's In the Box

| Capability | Status |
|---|---|
| OpenAI-compatible surface (`/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`, `/v1/models`) | ✅ |
| Anthropic-compatible surface (`/v1/messages`) | ✅ |
| Multi-tenancy — tenants, agents, per-tenant upstreams and keys | ✅ |
| RBAC — roles, permissions, tenant members | ✅ |
| Admin auth — password login + OIDC SSO (Google, Microsoft Entra) | ✅ |
| Guardrails engine — regex / keyword / managed rules; block, rewrite, tag, shadow, substitute, log | ✅ |
| Managed detectors via ML sidecar (prompt injection today; pluggable via `analyzers.yml`) | ✅ |
| LLM router — per-tenant scoring, fallback, circuit breaker | ✅ |
| Observability — OTel traces to ClickHouse + Jaeger; TTFT, TPS, token counts | ✅ |
| `aigw` governance CLI — SSO login, key lifecycle, SDK key injection | ✅ |
| Admin UI — tenants, agents, keys, upstreams, guardrails, traces, members | ✅ |
| PII vault / masking | 🚧 UI stub, backend WIP |
| Document security (ClamAV, VirusTotal, inspector) | 📋 Planned |
| Rate limiting / quotas | 📋 Planned |

---

## Architecture

One binary, two HTTP servers, one shared router instance:

| Server | Port | Purpose |
|---|---|---|
| **Proxy** | `8080` | Hot path — authenticate, guard, route, forward, trace |
| **Admin** | `8081` | Management API — tenants, agents, keys, upstreams, rules, traces, router metrics |

```
Client → proxy:8080
  → Authenticate (API key → Redis cache → Postgres fallback)
  → agent/trace context headers (X-Agent-Id, X-Thread-Id, X-Session-Id)
  → guardrails engine (input scope) → block / rewrite / tag / log
  → llmrouter.Pick(tenantID, model) → best available upstream
  → reverse proxy / streaming passthrough
  → guardrails engine (output scope)
  → OTel span end (TTFT, TPS, token counts) → collector → ClickHouse + Jaeger
```

Routing scores endpoints by `w1×(1/TTFT_P99) + w2×avgTPS + w3×(1-errorRate)`,
with a per-endpoint circuit breaker (`Closed → Open → Half-Open → Closed`).
Endpoint selection is tenant-scoped and matches the model name exactly.

---

## Repository Layout

```
gateway/
├── backend/            Go — proxy + admin servers, guardrails, router
│   ├── cmd/gateway/    Entry point
│   ├── internal/       admin, auth (+oidc), db, guardrails, proxy, router,
│   │                   secllm, inference, clickhouse, telemetry, providers
│   ├── services/       mlrunner — Python ML inference sidecar
│   └── deployments/    docker-compose, otel-collector configs
├── cli/                Go — the aigw governance CLI
├── frontend/           React 19 + Vite admin UI
├── docs/               VitePress documentation site
└── landing/            Marketing site (not part of the gateway runtime)
```

---

## Configuration

The backend is configured entirely by environment variables; see
[`backend/.env.example`](backend/.env.example).

| Variable | Default | Description |
|---|---|---|
| `PROXY_PORT` | `8080` | OpenAI/Anthropic-compatible proxy port |
| `ADMIN_PORT` | `8081` | Admin API port |
| `POSTGRES_DSN` | `postgres://gateway:gateway@localhost:6432/gateway?sslmode=disable` | Postgres DSN |
| `REDIS_ADDR` | `localhost:6379` | Redis address (API key cache) |
| `ADMIN_EMAIL` | `admin@example.com` | Super admin seeded on first boot |
| `ADMIN_PASSWORD` | _(empty — login disabled if unset)_ | Super admin password (bcrypt-hashed in DB) |
| `JWT_SECRET` | _(random — tokens invalidated on restart)_ | HS256 signing key for admin sessions |
| `JWT_TTL_HOURS` | `24` | Session token lifetime |
| `API_KEY_CACHE_TTL_SEC` | `300` | Redis TTL for validated API keys |
| `OTEL_COLLECTOR_ADDR` | `localhost:4317` | OTLP gRPC endpoint |
| `CLICKHOUSE_ADDR` / `_USER` / `_PASSWORD` / `_DB` | `localhost:9001` / `default` / `otel` / `otel` | Trace analytics store; analytics degrade to zeros if unreachable |
| `MLRUNNER_ENABLED` | `true` | Enable managed ML detectors |
| `MLRUNNER_BASE_URL` | `http://localhost:8082` | ML sidecar address |
| `MLRUNNER_THRESHOLD` | `0.85` | Min score to flag prompt injection |
| `MLRUNNER_TIMEOUT_MS` | `200` | Per-call sidecar timeout |
| `OIDC_REDIRECT_BASE_URL` | `http://localhost:8081` | IdP callback base |
| `OIDC_FRONTEND_URL` | `http://localhost:5173` | Post-SSO landing page |
| `OIDC_GOOGLE_*`, `OIDC_MICROSOFT_*` | — | SSO providers; enabled only when both ID and secret are set |
| `UPSTREAM_N_*` | — | Upstream `N` (0–9): `KEY_ID`, `PROVIDER`, `MODEL`, `BASE_URL`, `API_KEY` |

Always set `JWT_SECRET` and `ADMIN_PASSWORD` explicitly outside local dev.
Keep port `8081` internal in production — the admin API is not for end users.

---

## Development

```bash
make test          # go test -race ./... (delegates to backend)
make test-cover    # coverage report
make lint          # go vet
make build         # compile
make docs-dev      # VitePress docs on :5173
```

Run the gateway locally without rebuilding images:

```bash
cd backend
make tidy          # once
make infra         # postgres + redis + clickhouse + otel + jaeger + mlrunner
make dev           # both servers via go run, loading .env
```

Admin UI:

```bash
cd frontend && pnpm install && pnpm dev    # :5173, proxies /api → :8081
```

> The compose `docs` service also binds `:5173`. Stop it (`docker compose stop docs`)
> before starting the admin UI dev server, or run Vite on another port.

See [CONTRIBUTING.md](CONTRIBUTING.md) for workflow and conventions.

---

## Documentation

**Live docs: <https://gateway.theslmcompany.com/docs/>**

The docs are not on a third-party host — they ship with the gateway. The `docs`
container (VitePress → nginx, [`docs/Dockerfile`](docs/Dockerfile)) is built by
[`backend/deployments/docker-compose.prod.yml`](backend/deployments/docker-compose.prod.yml),
and the frontend nginx proxies `/docs/` to it. Every deployment therefore serves
its own docs at `https://<your-gateway-host>/docs/`.

| Environment | Command | URL |
|---|---|---|
| Local dev | `make docs-dev` | `http://localhost:5173` |
| Local compose | `make up` (docs service) | `http://localhost:5173` |
| Production | `docker compose -f backend/deployments/docker-compose.prod.yml up` | `https://<host>/docs/` |

Source lives in [`docs/`](docs/). Code samples interpolate `{{GATEWAY_URL}}`,
set once in [`docs/.vitepress/config.ts`](docs/.vitepress/config.ts).

- [Quickstart](docs/quickstart.md) — self-host and send your first request
- [Integration Guide](docs/integration.md) — SDKs, IDEs, context headers
- [CLI](docs/cli.md) — `aigw` key injection and governance
- [Guardrails](docs/guardrails.md) · [Router](docs/router.md) · [Traces](docs/traces.md)
- [Tenants](docs/tenants.md) · [Agents](docs/agents.md) · [API Keys](docs/api-keys.md) · [Provider Keys](docs/upstreams.md) · [Members](docs/members.md) · [SSO](docs/sso.md)

---

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) and the
[Code of Conduct](CODE_OF_CONDUCT.md) before opening a pull request.

## Security

This is security middleware — **do not report vulnerabilities via public issues.**
See [SECURITY.md](SECURITY.md) for the private disclosure process.

## License

[Apache License 2.0](LICENSE).
