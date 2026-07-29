# Quickstart

Self-host AI Gateway and send your first governed LLM request.

::: info Who this is for
Operators standing up their own gateway. If someone already runs a gateway for
you, skip to [Getting Started](/getting-started) (admin UI) or the
[Integration Guide](/integration) (point your app at it).
:::

---

## Prerequisites

| Need | Why |
|---|---|
| Docker + Docker Compose | Runs the gateway, Postgres, Redis, ClickHouse, OTel collector, Jaeger |
| `jq` | Parses JSON in the commands below |
| An upstream provider key | OpenAI, Anthropic, or any OpenAI-compatible endpoint |

Go 1.22+ and Node 20+/pnpm are only needed for local development — not for this
quickstart.

---

## 1. Clone and configure

```bash
git clone https://github.com/theslmcompany/gateway.git
cd gateway/backend
cp .env.example .env
```

Open `.env` and set four values:

```ini
ADMIN_EMAIL=admin@example.com          # super admin, seeded on first boot
ADMIN_PASSWORD=<pick-a-strong-one>     # login is disabled while this is empty
JWT_SECRET=<random-32-bytes>           # `openssl rand -hex 32`

UPSTREAM_0_KEY_ID=openai-primary
UPSTREAM_0_PROVIDER=openai
UPSTREAM_0_MODEL=gpt-4o
UPSTREAM_0_BASE_URL=https://api.openai.com
UPSTREAM_0_API_KEY=sk-<your-provider-key>
```

::: warning Rotate the shipped sample values
`.env.example` is a template. Never reuse its placeholder credentials in a real
deployment, and never commit your `.env`.
:::

Add more upstreams by incrementing the index (`UPSTREAM_1_*`, `UPSTREAM_2_*`, …).
Two upstreams with the same model give you automatic fallback.

---

## 2. Start the stack

```bash
make up
```

This starts the gateway plus Postgres, Redis, ClickHouse, the OTel collector,
Jaeger, and the ML sidecar used by managed detectors.

::: warning First run is not 60 seconds
The ML sidecar image bundles torch + transformers and downloads a model —
budget 5–10 minutes the first time. Every run after that starts in seconds.
To skip it entirely, set `MLRUNNER_ENABLED=false` in `.env` (regex and keyword
guardrails still work; managed detectors are disabled).
:::

Check it is up:

```bash
curl -s localhost:8081/health     # admin API
curl -s localhost:8081/ready      # dependencies reachable
```

| Port | Service |
|---|---|
| `8080` | Proxy — the OpenAI/Anthropic-compatible surface your apps call |
| `8081` | Admin API — management only, keep internal in production |
| `8082` | ML inference sidecar |
| `16686` | Jaeger UI |

---

## 3. Mint a gateway API key

The first boot seeds a `default` tenant. Log in as the super admin, find the
tenant, and create a key:

```bash
export ADMIN_EMAIL=admin@example.com
export ADMIN_PASSWORD=<the-one-you-set>

TOKEN=$(curl -s localhost:8081/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" | jq -r .token)

TENANT=$(curl -s localhost:8081/api/v1/tenants \
  -H "Authorization: Bearer $TOKEN" | jq -r '.[0].ID')

KEY=$(curl -s localhost:8081/api/v1/tenants/$TENANT/keys \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"quickstart"}' | jq -r .key)

echo $KEY     # sk_... — shown once, store it now
```

Gateway keys start with `sk_`. They are hashed at rest; the plaintext is
returned only at creation time.

---

## 4. Send your first request

```bash
curl localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}'
```

::: danger Model names must match exactly
The router picks endpoints by `(tenant, model)` with a literal string match.
If you configured `UPSTREAM_0_MODEL=gpt-4o`, then `gpt-4o-mini` returns
"no endpoint available". Register every model you intend to call.
:::

---

## 5. See the trace

Open Jaeger at `http://localhost:16686` and select the `ai-gateway` service.
Each request carries time-to-first-token, tokens per second, token counts, and
the upstream that served it. Traces also land in ClickHouse, which is what the
admin UI's **Traces** page queries.

Attach context to make traces useful:

```bash
curl localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $KEY" \
  -H 'X-Agent-Id: support-bot' \
  -H 'X-Thread-Id: conv-42' \
  -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}'
```

See [Integration Guide → Context Headers](/integration#context-headers).

---

## 6. Point a real app at it

Nothing in your application code changes — only the base URL and the key.

::: code-group

```bash [OpenAI SDK]
export OPENAI_BASE_URL=http://localhost:8080/v1
export OPENAI_API_KEY=$KEY
python app.py
```

```bash [Anthropic SDK / Claude Code]
export ANTHROPIC_BASE_URL=http://localhost:8080
export ANTHROPIC_API_KEY=$KEY
claude
```

```bash [aigw CLI]
aigw config set endpoint http://localhost:8081
aigw config set proxy    http://localhost:8080
aigw login
eval "$(aigw env openai)"
python app.py
```

:::

The [`aigw` CLI](/cli) is the recommended path for teams: developers log in with
SSO, and the CLI mints and injects a gateway key so provider credentials never
reach a laptop. It requires SSO to be configured — see [SSO](/sso).

---

## 7. Run the admin UI (optional)

```bash
cd frontend
pnpm install
pnpm dev          # http://localhost:5173, proxies /api → :8081
```

Log in with `ADMIN_EMAIL` / `ADMIN_PASSWORD`.

::: tip Port conflict
The compose `docs` service also binds `:5173`. Run `docker compose stop docs`
first, or start Vite on another port.
:::

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `invalid credentials` on login | `ADMIN_PASSWORD` was empty at first boot, so no admin was seeded | Set it in `.env`, then `make down && make up` |
| `no endpoint available: <model>` | No upstream registered for that exact model on that tenant | Add `UPSTREAM_N_MODEL`, or add the model on the **Provider Keys** page |
| `401` from the proxy | Using a provider key instead of the gateway key | Use the `sk_...` key from step 3 |
| Gateway container restarting | Postgres or the ML sidecar not healthy yet | `make logs`; the sidecar takes ~60s to become healthy on first boot |
| Traces missing in the UI | ClickHouse unreachable — analytics degrade to zeros | Check `CLICKHOUSE_ADDR`; the gateway logs `clickhouse unavailable` at startup |

Useful commands:

```bash
make logs      # tail everything
make down      # stop the stack
make downv     # stop and drop volumes (wipes tenants, keys, traces)
```

---

## Next steps

- [Guardrails](/guardrails) — block, rewrite, or log risky prompts and responses
- [Router](/router) — fallback, scoring, and circuit breakers
- [Tenants](/tenants) and [Members](/members) — isolate teams, assign roles
- [SSO](/sso) — Google Workspace or Microsoft Entra login for admins and the CLI
- [API Keys](/api-keys) — scoping and rotation
