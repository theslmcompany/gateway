# AI Gateway

**Secure, observable middleware for agentic AI systems.**

AI Gateway sits between your application and LLM providers (OpenAI, Anthropic, Gemini). It gives you routing, guardrails, observability, and multi-tenancy — without changing a line of your existing code. Drop-in compatible with OpenAI SDKs and Anthropic-native clients.

---

## What It Does

| Capability | What you get |
|---|---|
| **Smart Routing** | Automatic fallback between providers. Circuit breakers prevent cascading failures. Live endpoint health scores. |
| **Guardrails** | Block, log, or rewrite requests based on regex rules or managed detectors (prompt injection, secret leaks, toxicity). |
| **Observability** | Every request traced end-to-end. TTFT, TPS, cost tracking per tenant, agent, and model. OTel-native. |
| **Multi-Tenancy** | Isolated namespaces for teams, projects, or customers. Per-tenant API keys, upstreams, and guardrail rules. |
| **Drop-in Compatible** | Works with `openai` Python/Node SDKs, Cursor, Continue, Cline, and Claude Code. No code changes required. |

---

## How It Works

```
Your App  →  AI Gateway  →  Upstream LLM
(OpenAI SDK)   ├─ Authenticate API key
               ├─ Evaluate guardrail rules
               ├─ Select best endpoint (router)
               ├─ Forward request
               ├─ Track latency, tokens, cost
               └─ Return response
```

---

## Quick Links

- **[Quickstart](/quickstart)** — Self-host the gateway and send your first request
- **[Getting Started](/getting-started)** — Log in, understand the UI
- **[Integration Guide](/integration)** — Connect your app in 5 minutes
- **[CLI (`aigw`)](/cli)** — SSO login and gateway-key injection for any SDK
- **[API Keys](/api-keys)** — Create your first key
- **[Provider Keys](/upstreams)** — Connect your LLM providers
