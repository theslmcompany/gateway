# AI Gateway MVP: Epics

Based on the actual state of the backend (Guardrails engine is fully functional and wired in), we have restructured the remaining MVP scope into 5 distinct Epics. They are ordered by priority, focusing on shipping the highest-leverage AI-specific features first.

**Legend:** `[x]` done · `[~]` partial (see note) · `[ ]` not started.
_Last reconciled against source: 2026-07-29 (branch `feature/actions-firebase`)._

## Rollup

| Epic | Done | Partial | Open | Notes |
|---|---|---|---|---|
| E0 Identity Foundation | 2 | 2 | 6 | OIDC login flow live; sessions + audit missing |
| E1 Perf Benchmarking | 0 | 0 | 3 | Now urgent — E2 added uncached sidecar hop |
| E2 Security LLM | 2 | 1 | 1 | Only caching left |
| E3 PII Masking / Vault | 0 | 0 | 5 | Nothing started |
| E4 Document Security | 0 | 0 | 5 | Nothing started |
| E5 Access Mgmt & HITL | 0 | 1 | 3 | Keys done, rate limiting absent |
| E6 UI/UX Integration | 0 | 1 | 2 | Blocked on E3/E4/E5 |
| E7 Security & Compliance | 0 | 0 | 4 | Nothing started |
| E8 Provider & Routing | 0 | 0 | 4 | Nothing started |
| E9 User Auth & Rotation | 1 | 3 | 9 | SSO half done, token lifecycle untouched |
| E10 Agent Auth | 0 | 0 | 10 | Nothing started |
| E11 `aigw` CLI | 3 | 2 | 9 | Usable CLI; no hardware keys or distribution |
| E12 MCP Admin Server | 0 | 0 | 10 | Nothing started |
| **E13 Vuln Scan & Remediation** | 8 | 3 | 0 | CI gate green; **gitleaks disabled**; 2 leaked secrets need rotation |
| **Total** | **16** | **14** | **70** | 100 stories |

**Highest-leverage next moves:**
1. **Rotate the two credentials in local git history** (Google OAuth client secret, `JWT_SECRET`) — see the escalation under E13.S11. Not exposed on any remote, but both are live.
2. **E9.S1/S2/S3** — HS256 shared-secret JWTs with no `jti`, refresh, or revocation is now the biggest security gap in shipped design; it also unblocks most of E11. Rotating the `JWT_SECRET` above is a stopgap for the same weakness.
3. **E2.S4 + E1.S1** — the secllm hot-path call is unmeasured and uncached. Benchmark, then cache.
4. **E0.S4** — bearer JWT handed to the browser in a URL fragment; cookie sessions close this out.

---

## Epic 0: Identity Foundation (SSO / IdP Integration)
**Goal:** No user passwords ever stored in the gateway. All human identity sourced from external IdP via OIDC (primary) or SAML (enterprise). Prerequisite for Epics 5, 9, 11.
**Status:** In Progress — OIDC auth-code + PKCE flow, JIT provisioning, and auth middleware shipped (`5411693`). Sessions, audit, and break-glass remain.

**Approach:** Phase 1 — `coreos/go-oidc` v3 + `golang.org/x/oauth2` for OIDC. Covers Google Workspace, Microsoft Entra/Azure AD, Okta-OIDC, Auth0, GitHub, generic OIDC providers. Phase 2 — `crewjam/saml` if enterprise customers demand SAML. Re-evaluate WorkOS / Zitadel only if SCIM + many SAML connections needed.

**Package layout:**
```
internal/auth/
  oidc/        # provider registry, discovery, JWKS, ID-token verification
  saml/        # (phase 2) SP middleware, assertion parsing
  session/     # redis-backed browser sessions (httpOnly, SameSite=strict)
  middleware/  # http.Handler wrappers: requires-auth, requires-role
  jit/         # JIT user provisioning from IdP claims
  tenant/      # per-tenant IdP config CRUD
```

**Actual layout shipped:** `internal/auth/oidc/{provider,state,handlers}.go` (registry + discovery, Redis PKCE/nonce state, login/callback + JIT). Auth + RBAC middleware lives in `internal/admin/server.go` (`authMiddleware`, `RequirePermission`), not a separate `middleware/` package. No `saml/`, `session/`, `jit/`, or `tenant/` packages yet.

**Stories:**
- [~] **E0.S1: Tenant IdP Config:** Admin UI + API to register OIDC IdP per tenant (issuer, client_id, client_secret, redirect_uri, scopes, claim mappings). Validate via discovery doc on save.
    - _Partial:_ providers configured **globally** via env (`config.OIDCConfig`) and discovered at boot in `oidc.NewRegistry`; `GET /auth/oidc/providers` lists enabled ones. No per-tenant config, no CRUD API/UI, no claim mappings.
- [x] **E0.S2: OIDC Auth-Code + PKCE Flow:** `/auth/login` initiates flow w/ state + PKCE. `/auth/callback` verifies ID token via `coreos/go-oidc` verifier, mints browser session. State/nonce in Redis w/ 10min TTL.
    - _Shipped as_ `/auth/oidc/{provider}/login` + `/auth/oidc/{provider}/callback`. S256 PKCE + nonce in Redis (`oidc/state.go`), ID token verified via `coreos/go-oidc`. Mints a **JWT**, not a cookie session (see E0.S4). Loopback redirect gated against open-redirect for CLI login.
- [~] **E0.S3: JIT User Provisioning:** First successful SSO creates user record. Roles derived from IdP group claims via configurable mapping (e.g., `groups: ["aigw-admins"] → role: admin`).
    - _Partial:_ `provisionUser` handles identity lookup → email account-merge → create user w/ empty `PasswordHash`. **Group-claim → role mapping not implemented.**
- [ ] **E0.S4: Redis Session Management:** httpOnly, SameSite=strict cookies. Idle (30min) + absolute (12h) timeouts. `/auth/logout` revokes session + best-effort IdP RP-initiated logout.
    - _Blocked on design:_ current flow hands a bearer JWT to the browser (URL fragment). No cookie session, no idle/absolute timeout, no logout revocation. Redis is used only for PKCE state.
- [x] **E0.S5: Auth Middleware:** `RequiresAuth` + `RequiresRole(...)` Go middleware. Injects user context into request. Standard 401/403 responses.
    - _Shipped as_ `authMiddleware` + `RequirePermission(perm)` in `internal/admin/server.go` — permission-based rather than role-based, SuperAdmin bypass included.
- [ ] **E0.S6: Bootstrap Magic-Link Admin:** Onboarding flow for a new tenant's first admin — one-time signed link emailed, no password. Auto-disabled once SSO is configured.
- [ ] **E0.S7: Break-Glass WebAuthn Admin:** Per-tenant local admin secured by WebAuthn / passkey only (no password). For IdP outages. Library: `go-webauthn/webauthn`. Heavily audited.
- [ ] **E0.S8: Audit Log:** `auth.login`, `auth.logout`, `auth.idp.config.change`, `auth.breakglass.use` spans + persisted trail. SOC2-grade.
- [ ] **E0.S9: SAML Support (Phase 2, Stretch):** `crewjam/saml` SP integration for enterprise IdPs (ADFS, OneLogin, Okta-SAML). Per-tenant SP metadata. Skip until customer demands.
- [ ] **E0.S10: SCIM 2.0 Sync (Phase 2, Stretch):** Inbound SCIM endpoint for IdP-pushed user/group lifecycle. De-provision on offboarding. Skip until enterprise needs it.

---

## Epic 1: Performance Benchmarking & De-risking
**Goal:** Ensure the proxy hot-path can handle payload inspection without unacceptable latency overhead, especially for streaming responses.
**Status:** Ready for Development — **now urgent.** The secllm sidecar call (E2.S1) is uncached and sits on the hot path; no latency data exists yet.

**Stories:**
- [ ] **E1.S1: Synthetic Benchmark Script:** Build a script that pumps streaming traffic through the proxy and measures P50/P99 TTFT and TPS.
- [ ] **E1.S2: Proxy Latency Budget:** Instrument `handlers.go` with synthetic CPU load to determine the maximum millisecond budget the Guardrails/PII engine has per chunk.
- [ ] **E1.S3: Guardrails Optimization:** (If necessary based on S1) Optimize the current regex-based guardrails engine for streaming chunks.

---

## Epic 2: Dedicated Security LLM Pipeline
**Goal:** Implement a fast, fine-tuned security LLM to perform real-time semantic analysis for prompt injection, data poisoning, and complex issue patterns that regex cannot catch.
**Status:** In Progress — classifier live in the hot-path (`f38394d`, `c9f5430`, `dd298f6`). Caching outstanding.

**Stories:**
- [x] **E2.S1: Security LLM Orchestration:** Integrate the dedicated security LLM call into the `internal/proxy` hot-path.
    - `internal/secllm.Detector` called from `proxy/handlers.go` for input + tool scopes; emits `secllm.classify` spans w/ `secllm.score`, `secllm.latency_ms`, `secllm.error`. Model served by the `services/mlrunner` Python sidecar via `internal/inference`.
- [x] **E2.S2: Semantic Threat Detection:** Fine-tune or configure the system prompt to detect advanced prompt injections and jailbreaks.
    - Uses fine-tuned classifier `patronus-studio/wolf-defender-prompt-injection` (text-classification, not prompt-based). Threshold configurable on the detector.
- [~] **E2.S3: Data Poisoning Detection:** Implement analysis of inbound RAG context/tool outputs to detect data poisoning attempts.
    - _Partial:_ tool-output content is classified (`secllm.scope=tool`), but only against the injection model. No dedicated poisoning analysis of RAG context.
- [ ] **E2.S4: Caching Layer:** Implement strict caching (e.g., exact hash match) for security LLM verdicts to minimize latency.
    - Every request hits the sidecar today — highest-leverage remaining latency work; pairs with E1.S1.

---

## Epic 3: Bidirectional PII Masking & Session Vault
**Goal:** Implement the core data privacy feature: detecting, tokenizing, and unmasking PII in real-time.
**Status:** Ready for Development

**Stories:**
_Nothing shipped: no `internal/vault` package. Only regex **detection** exists today — `pii-leakage-output` in `guardrails/managed.go` — which blocks/flags but never tokenizes or unmasks. `services/mlrunner/analyzers.yml` has a commented-out `ner-pii` analyzer stub ready to enable for S2._

- [ ] **E3.S1: Redis Session Vault:** Implement the `internal/vault` package to store and retrieve ephemeral mapping tokens (e.g., `[PII_EMAIL_1]` -> `actual@email.com`) with a 24h TTL.
- [ ] **E3.S2: Inbound Masking Pipeline (Regex/NER):** Intercept requests in `internal/proxy`, detect entities, store them in the vault, and rewrite the payload before it hits the LLM.
- [ ] **E3.S3: Outbound Unmasking (Non-Streaming):** Scan complete LLM responses, identify vault tokens, dereference them, and rewrite the final response.
- [ ] **E3.S4: Outbound Unmasking (Streaming):** Implement a chunk-aware scanner in `handlers.go` that can identify tokens split across stream frames and unmask them smoothly.
- [ ] **E3.S5: Token Index API:** Ensure the API can return the `pii_map` for client-side rendering when `pii_response_mode: tokens` is set.

---

## Epic 4: Document Security Pipeline
**Goal:** Ensure all files/URLs processed by agents are free of malware and semantic threats.
**Status:** Backlog

**Stories:**
_Nothing shipped: no `internal/docsec` package, no ClamAV service in Docker Compose, no VirusTotal client._

- [ ] **E4.S1: Pipeline Orchestration:** Create the `internal/docsec` package to route files through the 3 stages before content extraction.
- [ ] **E4.S2: ClamAV Integration (Stage 1):** Add the ClamAV sidecar to Docker Compose and implement the streaming signature scan.
- [ ] **E4.S3: VirusTotal Integration (Stage 2):** Implement the VT API check for external URLs/hashes.
- [ ] **E4.S4: LLM Payload Inspector (Stage 3):** Implement the prompt injection and semantic threat detection call to the dedicated inspector LLM model, including caching.
- [ ] **E4.S5: OTel Tracing:** Emit `doc.scan.*`, `doc.vt.*`, and `doc.inspector.*` spans.

---

## Epic 5: Access Management & HITL Authentication
**Goal:** Provide secure user administration and human-in-the-loop approvals for agent tools.
**Status:** Backlog

**Stories:**
- [~] **E5.S1: Self-Service API Keys & Rate Limiting:** Implement rate limiting (TPS, TPM, RPM, RPD) logic on the proxy path using Redis.
    - _Partial:_ API key lifecycle done — admin API (`/tenants/{id}/keys` list/create/revoke, `keys.read`/`keys.manage` perms), `APIKeys.tsx` UI, and `aigw keys list|create|revoke`. **No rate limiting of any kind on the proxy path** (no TPS/TPM/RPM/RPD). Recommend splitting into E5.S1a (keys, done) / E5.S1b (rate limiting).
- [ ] **E5.S2: MFA & Re-Captcha:** Add Re-Captcha verification to login and support TOTP/Email OTP for admin actions.
- [ ] **E5.S3: HITL Tool Approvals:** Intercept sensitive tool calls and hold the LLM trace open while waiting for human MFA verification via the admin UI.
- [ ] **E5.S4: HRMS Sync (Stretch):** Basic polling ingestion of users from BambooHR/Workday.

---

## Epic 6: UI/UX Integration & Polish
**Goal:** Wire up all new backend APIs to the React frontend.
**Status:** Blocked (Depends on Epics 3, 4, 5)

**Stories:**
- [~] **E6.S1: PII Vault Dashboard:** Connect the React UI to manage PII entities and view Session Vault mappings (if permitted by policy).
    - _Partial:_ `frontend/src/pages/PiiVault.tsx` + sidebar entry exist but render **mock data** (`data/mock.ts`). Wiring blocked on E3.
- [ ] **E6.S2: Document Security Policy UI:** Add UI settings to configure ClamAV bypasses, VT thresholds, and Inspector fail-open/fail-closed states.
- [ ] **E6.S3: HITL Approval Queue:** Create a dashboard view where humans can review and approve pending tool calls.

---

## Epic 7: Security Testing & Compliance
**Goal:** Ensure the AI Gateway meets enterprise security standards, including SOC2 and HIPAA compliance requirements.
**Status:** Backlog

**Stories:**
_Nothing shipped. Note for S2: guardrail events already stream to ClickHouse via otelcol (`internal/clickhouse`, `telemetry/setup.go`) — that's an events trail, **not** an access-control/config-change audit log. `.github/workflows/` currently holds only Firebase deploy jobs; no SAST/DAST._

- [ ] **E7.S1: Penetration Testing:** Conduct automated and manual security testing on proxy and admin endpoints.
- [ ] **E7.S2: SOC2 Audit Preparation:** Implement comprehensive audit logging for all access control and configuration changes.
- [ ] **E7.S3: HIPAA Compliance Controls:** Ensure BAA (Business Associate Agreement) prerequisites are met, including end-to-end encryption and strict PHI (Protected Health Information) data handling within the vault.
- [~] **E7.S4: Vulnerability Scanning:** Integrate SAST and DAST tools into the CI/CD pipeline.
    - _SAST half satisfied by E13.S1 + E13.S9_ — `.github/workflows/security-scan.yml` runs gosec + staticcheck on both Go modules alongside the dependency gate. **DAST remains unbuilt** and is still uniquely scoped here.

---

## Epic 8: Advanced Provider & Routing Policies
**Goal:** Implement granular control over provider-specific features and complex routing strategies like provisioned throughput and priority-based failover.
**Status:** Backlog

**Stories:**
_Nothing shipped. `internal/router` does model→endpoint selection only; no priority/provisioned-throughput policy, no `thought_signature` passthrough._

- [ ] **E8.S1: Thought Signature Passthrough:** Ensure `thought_signature` and other provider-specific response headers/metadata are correctly preserved and passed to the client.
- [ ] **E8.S2: Provisioned Throughput Support:** Add support for routing to provisioned throughput endpoints (e.g., Azure OpenAI PTU or AWS Bedrock Provisioned Throughput) with specific rate-limit logic.
- [ ] **E8.S3: Priority Routing (Pay-as-you-go vs. Provisioned):** Implement routing policies that prioritize provisioned throughput and fallback to pay-as-you-go based on latency or cost settings.
- [ ] **E8.S4: Provider Metadata API:** Extend the Admin API to allow configuring advanced provider-specific metadata per endpoint.

---

## Epic 9: User Authentication & CLI Key Rotation
**Goal:** Eliminate long-lived user API keys. Humans authenticate via SSO/OAuth through a CLI. Short-TTL tokens minted, stored in OS keychain, auto-refreshed transparently. Root signing key lives in KMS — never in app memory.
**Status:** In Progress — SSO root-of-trust + CLI login shipped. The token *lifecycle* half (KMS, refresh, revocation) is untouched: the gateway still signs with an **HS256 shared secret** (`internal/auth/jwt.go`) and issues a single long-ish-lived token with no `jti` and no refresh.

**Stories:**
- [ ] **E9.S1: Root Signing Key in KMS:** Asymmetric JWT signing key (RS256 or EdDSA) provisioned in cloud KMS / Vault Transit. Gateway signs via KMS API; private key never leaves HSM. JWKS endpoint publishes public key. KMS-native rotation with overlapping key versions.
    - Current: symmetric HS256 w/ shared secret. No JWKS endpoint. Blocks S2/S3 and any third-party verification.
- [~] **E9.S2: Token Mint Service:** `/v1/auth/token` endpoint supporting `authorization_code` + `refresh_token` grants. JWT w/ jti, 15min default access TTL, 30-day rotating refresh.
    - _Partial:_ `auth.GenerateToken(userID, name, email, isSuperAdmin, secret, ttl)` mints on OIDC callback and `/auth/login`. No `/v1/auth/token` endpoint, no grant types, no `jti`, no refresh token.
- [ ] **E9.S3: Revocation & JTI Store:** Redis-backed jti tracking + revocation list. Proxy hot-path checks revocation per request. Refresh-token reuse detection triggers family revoke.
- [~] **E9.S4: SSO / IdP Integration:** OAuth2 / OIDC integration with Google, Okta, Azure AD, GitHub. IdP assertion is the root of trust — no shared user secret stored in gateway.
    - _Partial:_ **Google + Microsoft/Entra** shipped (`oidc/provider.go` `buildGoogle`, `buildMicrosoft`). Okta and GitHub not wired. Users created via SSO carry an empty `PasswordHash` — root-of-trust goal met for those providers.
- [x] **E9.S5: CLI `aigw login`:** Browser-based SSO flow (PKCE). Refresh token stored in OS keychain (macOS Keychain, Windows Credential Manager, libsecret on Linux). Fallback to encrypted file w/ passphrase.
    - `aigw login [--provider google|microsoft]` → loopback listener (`cli/internal/auth/loopback.go`) → credentials in `99designs/keyring`, encrypted-file fallback at `~/.config/aigw/keyring` w/ passphrase prompt. Caveat: stores the **session JWT** (no refresh token exists yet — see S2).
- [~] **E9.S6: CLI Auto-Refresh & UX:** Background refresh before TTL expiry. Transparent retry on 401. Commands: `aigw whoami`, `aigw logout`, `aigw token` (print short-lived token for piping).
    - _Partial:_ all three commands shipped, plus `Credentials.Expired()`. **No background refresh and no transparent 401 retry** — blocked on S2.
- [ ] **E9.S7: Per-User Rotation Policy:** Admin-configurable access TTL, refresh max-age, idle revoke, device binding. Per-tenant defaults.
- [ ] **E9.S8: Legacy Key Deprecation Path:** Existing long-lived user keys allowed but flagged in audit log + email warning. Force-migrate deadline per tenant.
- [ ] **E9.S9: Audit Log:** OTel spans `auth.user.mint`, `auth.user.refresh`, `auth.user.revoke` + persisted audit trail for SOC2.
- [ ] **E9.S10: Device-Bound Keys (Hardware POP):** CLI generates non-exportable keypair in Secure Enclave (macOS) / TPM (Windows/Linux) / Android Keystore at login. Public key registered w/ gateway, bound to user + device_id. Private key never leaves hardware.
- [ ] **E9.S11: DPoP Token Binding (RFC 9449):** Tokens include `cnf` thumbprint claim. Every mint/refresh and (configurable) proxy request requires DPoP proof signed by device key. Stolen token alone is useless without device.
- [ ] **E9.S12: Device Management:** `aigw devices list / revoke / rename` CLI commands. Admin UI to view + revoke devices per user. Audit log on register/revoke. Lost-device recovery via re-login from another device.
- [ ] **E9.S13: Software-Key Fallback:** Encrypted keypair on disk for environments without hardware enclave. Protected by OS keyring passphrase. Flagged as lower-assurance in audit; admin policy can disallow.

---

## Epic 10: Agent Authentication & Workload Identity
**Goal:** Eliminate long-lived API keys in agent / service deployments. SDK obtains short-TTL tokens via workload identity (cloud IAM, k8s, CI/CD OIDC) — zero secrets in code, env vars, or container images.
**Status:** Backlog

**Stories:**
_Nothing shipped. Agents authenticate with long-lived API keys today (`internal/auth/keys.go`, `/tenants/{id}/keys`) — exactly what this Epic replaces._

- [ ] **E10.S1: Workload Identity Federation Endpoint:** `/v1/auth/token` `client_credentials` + `urn:ietf:params:oauth:grant-type:token-exchange` grants. Accepts external OIDC assertions, returns short-lived gateway JWT.
- [ ] **E10.S2: Cloud IAM Trust Providers:** Configure trust for AWS IAM (sigv4 / IRSA), GCP service accounts (Workload Identity Federation), Azure Managed Identity. Gateway verifies cloud-issued tokens, mints its own.
- [ ] **E10.S3: Kubernetes Projected SA Tokens:** Trust `kubernetes.io` issuer per cluster. Pod's projected service-account token → gateway token. Audience-bound to gateway.
- [ ] **E10.S4: CI/CD OIDC Trust:** Trust GitHub Actions, GitLab CI, CircleCI, Buildkite OIDC issuers. Per-repo / per-workflow / per-branch claim matching. No long-lived CI secrets needed.
- [ ] **E10.S5: mTLS Client Auth:** SPIFFE/SPIRE-compatible mTLS option for on-prem / non-cloud workloads. Cert SAN → identity mapping.
- [ ] **E10.S6: Bootstrap Secret (Fallback):** For environments lacking workload identity: short-lived bootstrap secret issued by admin, single-use exchange for credentials, auto-rotates on use.
- [ ] **E10.S7: SDK Auto-Refresh (Python / Node / Go):** SDK detects environment (k8s, GHA, AWS, GCP, Azure) and auto-selects identity provider. Refreshes in background. Consumer code never handles tokens.
- [ ] **E10.S8: Identity-to-Policy Binding:** Map workload identities (e.g., `repo:org/foo:ref:refs/heads/main`) to rate limits, model allow-lists, guardrail policies.
- [ ] **E10.S9: Audit Log:** OTel spans `auth.agent.mint`, `auth.agent.exchange`, `auth.agent.revoke` w/ source issuer + claims for forensics.
- [ ] **E10.S10: Break-Glass Static Token:** Sealed-envelope long-lived token for incidents / unsupported envs. Requires admin MFA to issue, auto-expires 24h, heavily audited.

---

## Epic 11: `aigw` CLI Implementation
**Goal:** Ship a cross-platform Go CLI (`aigw`) for Windows/macOS/Linux that delivers Epic 9's user auth flow: SSO login, hardware-bound device keys, auto-refresh, token piping, device management. Single static binary, no runtime deps.
**Status:** In Progress — usable CLI shipped at `cli/` (scaffold, login, keyring, config, token piping, env/run). Hardware-key, DPoP, device, and distribution stories all blocked on Epic 9.

**Stories:**
- [x] **E11.S1: CLI Scaffold (Cobra + Viper):** Bootstrap repo layout. Commands: `login`, `logout`, `whoami`, `token`, `devices`, `config`, `version`. Shared HTTP client w/ retries + tracing.
    - `cli/` module w/ cobra v1.10 + viper v1.21. Shipped commands: `login`, `logout`, `whoami`, `token`, `config`, `version`, `session`, `tenants`, `keys`, `env`, `run`. Shared client in `cli/internal/client`. **Not shipped:** `devices` (see S8), and the client has no retry/tracing layer yet.
- [~] **E11.S2: OIDC / PKCE Login Flow:** Browser-based auth via `golang.org/x/oauth2` + `coreos/go-oidc`. Local loopback listener for callback. Handles headless mode (device-code grant) for SSH sessions.
    - _Partial:_ loopback listener shipped; the OAuth2/OIDC exchange is delegated to the **gateway** (`/auth/oidc/...`) rather than performed in the CLI. **No device-code grant → SSH/headless login unsupported.**
- [x] **E11.S3: OS Keyring Integration:** Use `99designs/keyring` to store refresh token in macOS Keychain / Windows Credential Manager / libsecret / KWallet. File-backed fallback w/ passphrase prompt.
    - `cli/internal/auth/store.go`. Also caches per-tenant proxy keys (`SaveProxyKey`/`LoadProxyKey`).
- [ ] **E11.S4: Hardware Key Generation:**
    - macOS: Secure Enclave via `Security.framework` (cgo) or `facebookincubator/sks`.
    - Windows: TPM 2.0 via `google/go-tpm` + Platform Crypto Provider.
    - Linux: TPM 2.0 via `go-tpm`; fallback to encrypted software key.
- [ ] **E11.S5: DPoP Proof Signing:** Generate DPoP JWT per request signed by device key. JWS via `go-jose/go-jose`. Nonce handling + replay protection.
- [ ] **E11.S6: Auto-Refresh Daemon Mode:** Background refresh before TTL expiry. Optional long-running mode (`aigw agent`) exposing local Unix socket / named pipe for SDK token retrieval.
- [~] **E11.S7: Token Piping & Shell Integration:** `aigw token` prints short-lived JWT for `curl` / scripts. Shell completions (bash/zsh/fish/pwsh). `eval $(aigw env)` for env-var export.
    - _Partial:_ `aigw token` and `eval "$(aigw env openai|anthropic|generic)"` shipped, plus extras not in the original scope — `aigw env --write .env` and `aigw run <provider> -- <cmd>`. Shell completions rely on cobra's default `completion` command; none packaged or documented.
- [ ] **E11.S8: Device Management Commands:** `aigw devices list / revoke / rename`. Pretty-print table + `--json` output.
- [ ] **E11.S9: Cross-Platform Release Pipeline:** `goreleaser` config.
    - Today: hand-rolled `cli/Makefile` (`build`/`install`, ldflags stamp `version`+`commit`). No goreleaser, no build matrix, no signing/notarization, no SBOM. Build matrix: linux/mac/windows × amd64/arm64. macOS code signing + notarization (Apple Developer ID). Windows Authenticode signing. SBOM + checksums.
- [ ] **E11.S10: Distribution Channels:** Homebrew tap, Scoop bucket, `.deb` / `.rpm` packages, direct download from `releases.aigw.io`. Install script (`curl ... | sh`) w/ checksum verify.
- [ ] **E11.S11: Self-Update:** `aigw update` command via `minio/selfupdate`. Signature verification against release public key. Opt-in auto-check on startup.
- [x] **E11.S12: Config Management:** `~/.config/aigw/config.yaml` for endpoint, default tenant, telemetry opt-out. `aigw config get/set`. Env var overrides (`AIGW_ENDPOINT`).
    - `cli/internal/config/config.go` — viper w/ `AIGW` env prefix + `AutomaticEnv` (`AIGW_ENDPOINT`, `AIGW_PROXY`, `AIGW_TENANT`). Adds a `proxy` key beyond the original scope. Telemetry opt-out key deferred to S14.
- [ ] **E11.S13: Diagnostics:** `aigw doctor` checks hardware key support, keyring access, network reachability, clock skew (critical for JWT). `aigw logs` tails local refresh daemon.
- [ ] **E11.S14: Telemetry (Opt-In):** Anonymous usage + crash reporting. OTel-compatible. Off by default; clear prompt on first run.

---

## Epic 12: MCP Admin Server (AI-Driven Administration)
**Goal:** Expose the admin API surface as an MCP (Model Context Protocol) server so operators can run admin activities — tenant config, rules, keys, upstreams, members, trace inspection — through an AI agent in natural language. The agent calls typed MCP tools; every call enforces the same JWT auth + `RequirePermission` RBAC as the HTTP admin API.
**Status:** Backlog. No `internal/mcp` package yet. Dependency partly satisfied — E0.S5 (`authMiddleware` + `RequirePermission`) is the exact bridge target for E12.S2, and every handler S3/S4/S5 wrap already exists in `internal/admin`.

**Approach:** New `internal/mcp` package. Streamable-HTTP MCP transport (`modelcontextprotocol/go-sdk` or `mark3labs/mcp-go`). MCP tools are thin wrappers over existing `internal/admin` service methods — no business logic duplication. Auth context (tenant + caller identity) flows from the MCP session into each tool call; RBAC checks unchanged. Read tools always on; mutating tools gated by policy + per-call confirmation.

**Package layout:**
```
internal/mcp/
  server/    # MCP transport, session lifecycle, tool registry
  tools/     # one file per tool group (tenants, rules, keys, upstreams, members, traces)
  auth/      # MCP-session -> auth.Claims bridge, RBAC enforcement
  policy/    # which tools are exposed, read-only vs mutating, confirmation gates
```

**Stories:**
- [ ] **E12.S1: MCP Server Scaffold:** Stand up `internal/mcp/server` with streamable-HTTP transport, tool registry, and session lifecycle. Mount under admin server (e.g. `/api/v1/mcp`). Health + capability handshake.
- [ ] **E12.S2: Auth & RBAC Bridge:** Map MCP session credentials (Bearer JWT) to `auth.Claims`; enforce `RequirePermission` per tool call. Reject unauthorized tool calls with structured MCP errors. SuperAdmin bypass preserved.
- [ ] **E12.S3: Read Tools:** Expose `list_tenants`, `get_tenant`, `list_rules`, `list_keys`, `list_upstreams`, `list_members`, `list_agents`, `get_guardrail_events` as MCP tools mapped to existing admin GET handlers.
- [ ] **E12.S4: Trace Inspection Tools:** `list_traces`, `get_trace_spans` MCP tools so the agent can debug guardrail/proxy behavior and answer "why was this request blocked".
- [ ] **E12.S5: Mutating Tools w/ Confirmation:** `create_rule`, `update_rule`, `delete_rule`, `create_key`, `revoke_key`, `create_upstream`, `update_upstream`, `delete_upstream`, `add_member`, `remove_member`, `update_tenant`. Each marked destructive; require explicit confirmation arg before execution.
- [ ] **E12.S6: Tool Policy & Exposure Config:** Per-tenant config to enable/disable the MCP server, allow-list tool groups, and force read-only mode. Mutating tools off by default.
- [ ] **E12.S7: Resources & Prompts:** Expose config schemas + guardrail-rule catalog as MCP resources. Ship prompt templates (e.g. "audit this tenant's rules", "diagnose blocked request") to steer the agent.
- [ ] **E12.S8: Audit Log:** OTel spans `mcp.tool.call`, `mcp.session.start`, `mcp.tool.denied` with caller identity, tool name, args digest, and verdict. SOC2-grade trail for every AI-initiated admin action.
- [ ] **E12.S9: Rate Limiting & Guardrails:** Apply rate limits per MCP session; run mutating tool args through the Guardrails engine to block prompt-injection-driven destructive calls.
- [ ] **E12.S10: Docs & Agent Onboarding:** Document connecting Claude Desktop / Claude Code / generic MCP clients. Example `.mcp.json`. Scoped-token issuance guide for the agent.

---

## Epic 13: Vulnerability Scanning & Dependency Remediation
**Goal:** Get to a known-clean dependency and code baseline, then keep it clean automatically. Scan every ecosystem in the repo (Go × 2 modules, JS × 3 workspaces, Python sidecar, container images), remediate what is reachable, and gate CI so regressions can't merge.
**Status:** **Remediation pass complete (2026-07-29).** Both Go modules are `govulncheck`-clean and SAST-clean, the Python sidecar is assessed and patched, all three JS workspaces pass a High-and-above audit gate, and `.github/workflows/security-scan.yml` blocks regressions. S8 (base images pinned to tags, not digests), S10 (Dependabot running, but no config file) and S11 (**gitleaks CI job currently disabled**) remain partial.

**Relationship to Epic 7:** E13 is the *do it now* remediation pass; E7.S4 (Vulnerability Scanning) is the compliance-framing of the same CI gate. E13.S1 now ships that gate — E7.S4's SAST half is satisfied by it. Do not build two pipelines. DAST is still unbuilt and stays scoped to E7.S4.

### Outcome (2026-07-29)

| Surface | Before | After |
|---|---|---|
| `backend/` (Go) | 13 reachable | **0 reachable**, gosec + staticcheck clean |
| `cli/` (Go) | 7 reachable | **0 reachable**, gosec + staticcheck clean |
| `frontend/` (JS) | 1 critical, 26 high | **0 / 0** — nothing allowlisted |
| `landing/` (JS) | 1 critical, 16 high | **0 / 0** gated; 1 high accepted (unreachable, documented) |
| `docs/` (JS) | 2 high, 3 moderate | gate passes; 4 dev-server-only findings accepted + documented |
| `services/mlrunner/` (Python) | **unassessed** | assessed via OSV, **0 known vulns** across all 8 pins |
| Container images | unscanned | Trivy wired into CI (**not yet executed** — see S8) |

**Bugs found during remediation that the baseline scan did not cover.** Dependency scanners only see dependencies; these came out of reading the code the upgrades touched.

| Severity | Issue | Fix |
|---|---|---|
| **High** | `db.Tenant.APIKey` — the plaintext of a live gateway proxy key — had no `json:"-"`, so `GET /tenants`, `GET /tenants/{id}` and both create paths returned a working credential to **any member of the tenant**, bypassing the `keys.read` / `keys.manage` permissions entirely. | `json:"-"` on the field + `TestTenantAPIKeyNeverSerialized` covering all four handlers (verified failing without the fix). The column itself should be dropped — `APIKeys` already stores the hash — tracked as a TODO in `models.go`. |
| **High** | mlrunner ran every model with `trust_remote_code=True` **by default**, executing arbitrary Python from the model repo inside the sidecar, from an unpinned mutable `main` ref. | Default flipped to `False` with a startup warning when enabled; `revision` pinning added and warned about when absent. Verified against the live repo: it declares no `auto_map` and ships no `.py` — a stock `ModernBertForSequenceClassification` checkpoint, so the flag was never needed. |
| Medium | Reflected XSS in the CLI loopback login page — the IdP's `error` query param was interpolated into HTML unescaped, on the origin that receives the session token. | `template.HTMLEscapeString` in `resultPage`. |
| Medium | `~/.config/aigw` created `0755`; it holds the encrypted-file keyring backend. | `0700`. |
| Low | `aigw logout` errored on the file keyring backend when nothing was stored — it returns `fs.ErrNotExist`, not `keyring.ErrKeyNotFound`. | `isNotFound` normalises both; covered by the new round-trip tests. |
| Low | Auth claims stored in `context` under a bare `"claims"` string key — any package could shadow them (SA1029). | Unexported `claimsCtxKey` type with `withClaims` / `claimsFrom` accessors. |

**Found by the CI gate itself, on its first run (2026-07-29).** Five of seven jobs failed initially. Two failures were defects in the workflow; the other three were genuine, and are the clearest argument for the gate existing:

| Job | Cause | Fix |
|---|---|---|
| `pip-audit` | **Real.** `starlette` 0.46.2 — pulled in transitively by `fastapi` — carries 9 advisories. The earlier OSV sweep queried only the *pinned* packages, so it never saw transitives; `pip-audit` resolves the full tree, which is exactly the gap it closes. | `fastapi` 0.115.12 → 0.140.13 (first line accepting `starlette >= 1.x`), `starlette` pinned to 1.3.1. |
| `trivy` (all) | **Workflow defect.** `aquasecurity/trivy-action@0.28.0` — the tags are `v`-prefixed, so every image job failed to resolve the action. | `@v0.36.0`. |
| `trivy` (backend) | **Real,** once the action resolved. `golang.org/x/crypto` v0.51.0, 9 HIGH SSH CVEs. `govulncheck` rates them unreachable (the gateway never uses `x/crypto/ssh`); trivy is package-level and does not care. | → v0.52.0 in both modules. |
| `trivy` (frontend, docs) | **Real.** `nginx:1.27-alpine` was badly stale — CRITICAL OpenSSL plus ~30 HIGH. `python:3.11-slim` carried 2 HIGH. | `nginx:1.31-alpine`, `python:3.12-slim` (all mlrunner pins verified to have 3.12 wheels). Both now scan clean. |
| `gitleaks` | **Workflow defect.** `gitleaks/gitleaks-action@v2` requires a paid `GITLEAKS_LICENSE` for org-owned repos and fails closed without one. | Run the MIT-licensed OSS binary directly. |

**Two image builds were broken by this pass and are now fixed.** Building the images to scan them surfaced both:
- `frontend/Dockerfile` copied only `package.json` + `pnpm-lock.yaml`, not `pnpm-workspace.yaml` — so the security `overrides` were absent in-container and `--frozen-lockfile` rejected the lockfile.
- Neither `frontend/` nor `docs/` had a `.dockerignore`, so `COPY . .` overwrote the freshly installed `node_modules` with the host's, whose pnpm symlinks point at a host-side store. Latent since before this pass — it happened to work until the dependency graph changed. Added, plus a pnpm pin in `docs/Dockerfile`, which used bare `corepack enable` and was silently resolving to pnpm 11 against a pnpm-10 lockfile.

### Dependabot reconciliation (2026-07-29)

GitHub reported **131 alerts on `master`** after the first push. That number predates this work — it reflects `master` before the remediation branch merged. Reconciled with `osv-scanner`, which like Dependabot ignores reachability and reports every advisory against a manifest (the reason it counts far more than `govulncheck`, `pnpm audit --audit-level high` or `pip-audit`, all of which filter):

| Manifest | Pre-remediation | After merge | Now |
|---|---|---|---|
| `backend/go.mod` | 51 | 3 | 1 |
| `backend/services/mlrunner/requirements.txt` | 75 | 2 | 0 |
| `cli/go.mod` | 6 | 1 | 0 |
| `frontend/pnpm-lock.yaml` | 70 | 0 | 0 |
| `landing/pnpm-lock.yaml` | 29 | 2 | 1 |
| `docs/pnpm-lock.yaml` | 5 | 4 | 4 |
| **Total** | **236** | **12** | **6** |

**A further four fixed in this pass** — all missed first time because the reachability-filtered gates rated them unreachable and stayed green:
- `klauspost/compress` 1.18.3 → 1.18.7, `golang.org/x/net` 0.55.0 → 0.56.0 (`x/crypto` came along to 0.53.0).
- `golang.org/x/text` 0.37.0 → 0.39.0 in **`cli/`** — the backend was bumped to 0.39.0 in the first pass but the CLI was left behind, and `govulncheck` reported 0 reachable there, so nothing flagged it.
- `idna` pinned to 3.18 in the sidecar. Subtle one: `pip-audit` reported clean because it resolves transitives the way pip does (newest compatible), while Dependabot and osv-scanner assume the **minimum** satisfying version — 3.9.0, which carries GHSA-65pc-fj4g-8rjx. An explicit pin makes the safe version true under every resolver. Worth remembering for any other unpinned transitive.
- `@babel/core` override in `landing` (dev-only, GHSA-4x5r-pxfx-6jf8).

**The remaining 6 are accepted, and will not clear by bumping:**

| Finding | Why it stays |
|---|---|
| `golang.org/x/crypto` GO-2026-5932 | **Unfixable.** Not a CVE — an advisory that `x/crypto/openpgp` is permanently unmaintained and unsafe by design. The affected range is `introduced: 0` with **no fixed version**, so every future bump keeps it. The gateway does not import openpgp; `govulncheck` confirms it unreachable. |
| `vite` ×3 + `esbuild` (`docs`) | vitepress 1.6.4 depends on `vite ^5.4.14`; 6.x/8.x switch to rolldown and break the build (verified), and vitepress 2 is alpha. Dev-server only — never `vitepress build`, never production. |
| `react-router` GHSA-qwww-vcr4-c8h2 (`landing`) | RSC-mode only; `landing` is a client-side `BrowserRouter` SPA. Fix requires the react-router 8 major. The migration is small for this surface (`BrowserRouter`/`Routes`/`Route`/`Link`) if the alert is worth closing. |

**Corrections to the baseline's JS analysis.** "All transitive via `firebase`" was wrong in two places: `axios` (6 highs) and `react-router` (1 high) are **direct runtime dependencies** of `frontend` and `landing` respectively, not tree-shaken transitives. Both were missed because the analysis stopped at the most common root. The firebase claim itself held up — the vulnerable subtrees are confirmed absent from `frontend/dist` and `landing/dist`.

### Baseline scan (2026-07-28)

Tooling used: `govulncheck` (via `go run golang.org/x/vuln/cmd/govulncheck@latest`), `pnpm audit`. Nothing was installed permanently — no scanner is in the repo or CI today.

| Surface | Result |
|---|---|
| `backend/` (Go) | **13 reachable** vulns from 5 modules + stdlib (plus 7 imported-not-called, 24 in required modules) |
| `cli/` (Go) | **7 reachable** vulns — stdlib + `jose2go` |
| `frontend/` (JS) | 1 critical, 26 high, 39 moderate, 4 low — **all transitive via `firebase`** |
| `landing/` (JS) | 1 critical, 16 high, 11 moderate, 1 low — same `firebase` chain |
| `docs/` (JS) | 2 high, 3 moderate — `vite`, `postcss`, `esbuild`, `launch-editor` (build-time only) |
| `services/mlrunner/` (Python) | **NOT SCANNED** — `pip-audit` aborted locally (venv `ensurepip` SIGABRT). Unknown risk. |
| Container images | **NOT SCANNED** — no image scanner available |

**Reachable Go findings (backend), highest-signal first:**

| ID | Module | Have → Fixed | Note |
|---|---|---|---|
| GO-2026-5775 / 5777 | `go-chi/chi/v5` | 5.1.0 → 5.3.0 | **IP spoofing via `X-Forwarded-For`. Genuinely reachable — `middleware.RealIP` is active at `internal/proxy/server.go:55`.** Highest-priority item in this epic. |
| GO-2026-5004 | `jackc/pgx/v5` | 5.6.0 → 5.9.2 | SQL injection via dollar-quoted-literal placeholder confusion; reached through `gorm.Open` |
| GO-2026-6061 | `google.golang.org/grpc` | 1.80.0 → 1.82.1 | xDS RBAC + HTTP/2 server; reached via OTel log exporter |
| GO-2026-4918 | `golang.org/x/net` | 0.52.0 → 0.53.0 | HTTP/2 infinite loop on bad `SETTINGS_MAX_FRAME_SIZE` |
| GO-2026-5970 | `golang.org/x/text` | 0.35.0 → 0.39.0 | Infinite loop on invalid input |
| GO-2026-5158 | `go.opentelemetry.io/otel` | 1.43.0 → 1.44.0 | Uncapped baggage header length |
| GO-2026-5856 | stdlib `crypto/tls` | go1.26.2 → go1.26.5 | ECH privacy leak |
| GO-2026-5039 / 5037 | stdlib `net/textproto`, `crypto/x509` | → go1.26.4 | |
| GO-2026-4980 / 4982 / 4971 | stdlib `html/template`, `net` | → go1.26.3 | |

**CLI:** `dvsekhvalnov/jose2go` 1.5.0 → 1.7.0 (GO-2025-4123, GO-2023-2409), pulled in by `99designs/keyring` (E11.S3). Same stdlib set as backend.

**JS nuance — do not panic-patch:** every critical/high in `frontend` and `landing` arrives through `firebase` → `@firebase/database` (`websocket-driver`) and `@firebase/firestore` (`@grpc/grpc-js`, `protobufjs`). Both apps import **only** `firebase/app` and `firebase/analytics` (`src/firebase.ts`), so the vulnerable subtrees are tree-shaken out of the shipped bundle. Verify against the built output before spending effort; a `firebase` minor bump or a pnpm `overrides` entry is likely sufficient. `docs/` findings are dev-server/build-time only.

**Stories:**
- [x] **E13.S1: CI Vulnerability Gate:** Add a `security-scan` GitHub Actions workflow running `govulncheck ./...` in `backend/` and `cli/`, `pnpm audit` in all three JS workspaces, and `pip-audit` for the mlrunner. Fail on reachable High/Critical; report-only for unreachable. Runs on PR + nightly. **Satisfies E7.S4.**
    - `.github/workflows/security-scan.yml` — 7 jobs on push/PR/nightly: `govulncheck` and `gosec`+`staticcheck` (matrixed over both Go modules), `pnpm audit --audit-level high` (matrixed over all three workspaces, plus a non-blocking full report), `pip-audit`, Trivy, and `gitleaks`. Accepted risks are allowlisted **in-repo** next to their justification (`auditConfig.ignoreGhsas`, `#nosec` comments), never muted in the workflow.
- [x] **E13.S2: Go Toolchain & stdlib Upgrade:** Bump to **go1.26.5**. Update `go.mod` `go` directive and `backend/Dockerfile` builder image.
    - Both modules on `go 1.26.5`; `backend/Dockerfile` builder → `golang:1.26-alpine`.
- [x] **E13.S3: Backend Module Upgrades:** `chi/v5` → 5.3.0, `pgx/v5` → 5.9.2, `grpc` → 1.82.1, `x/net` → 0.53.0, `x/text` → 0.39.0, `otel` → 1.44.0.
    - All applied (`x/net` landed on 0.55.0 via the otel SDK bump). Predicted `pgx`/`gorm` and `otel` breakage did not materialise — full backend suite passes unchanged.
- [x] **E13.S4: Harden `RealIP` Trust Boundary:** **Security bug, not just a version bump.**
    - chi 5.3.0 *deprecates* `RealIP` rather than fixing it, so the upgrade alone would have left the flaw in place. Replaced with `proxy.clientIPMiddleware` (`internal/proxy/clientip.go`), which resolves the client IP against a configured trust boundary and — unlike `RealIP` — never mutates `r.RemoteAddr`. Config: `TRUSTED_CLIENT_IP_HEADER` > `TRUSTED_PROXY_CIDRS` > `TRUSTED_PROXY_COUNT`, **defaulting to the TCP peer address with all forwarded-for headers ignored**. Malformed values fail startup rather than silently degrading. The resolved IP now lands on the `agent` span as `client.address` and is the natural key for E5.S1b rate limiting. Covered by `clientip_test.go` (including an explicit spoofing regression) and `config_test.go`.
- [x] **E13.S5: CLI Module Upgrades:** `jose2go` → 1.7.0, plus the S2 toolchain bump. Re-verify keyring file-backend encrypt/decrypt after.
    - Done. The re-verification was the point: it found the `aigw logout` bug above. `store_test.go` now round-trips both credentials and cached proxy keys through the file backend; `AIGW_KEYRING_BACKEND` was added to pin the backend for tests and headless machines.
- [x] **E13.S6: JS Workspace Remediation:** Confirm the `firebase` transitives are absent from built bundles, then bump/override. Document any accepted-risk leftovers.
    - Bundle check confirmed: no `websocket-driver`, `@grpc/grpc-js`, `protobufjs` or `faye-websocket` in `frontend/dist` or `landing/dist`. Pinned via `overrides` anyway so a future `firebase/firestore` import can't silently reintroduce them. Also bumped the **direct** deps the baseline missed (`axios` 1.15.2 → 1.18.1, `firebase`, `vite`, `shadcn`). Every workspace builds green.
    - _Accepted risk:_ `react-router` GHSA-qwww-vcr4-c8h2 (landing) — RSC-mode only, and landing is a `BrowserRouter` SPA; the fix is a react-router 8 major. Four `vite`/`esbuild` dev-server advisories (docs) — vitepress 1.6.4 pins `vite ^5.4.14`, 6.x/8.x switch to rolldown and break the build, vitepress 2 is alpha. All five are allowlisted with reasoning in the respective `pnpm-workspace.yaml`.
- [x] **E13.S7: Python Sidecar Scan:** Get `pip-audit` working against `services/mlrunner/requirements.txt`.
    - `pip-audit` still aborts locally (the `ensurepip` SIGABRT is in this machine's uv-managed CPython, not the tool) — assessed by querying OSV directly instead, which found **36 advisories across 4 of the 8 pins**, the worst surface in the repo. Remediated: `transformers` 4.52.3 → 5.14.1, `torch` 2.7.0 → 2.13.0, `sentencepiece` 0.2.0 → 0.2.1, `protobuf` 5.29.4 → 5.29.6. All 8 pins now return zero OSV findings and all support the Dockerfile's Python 3.11. The transformers 5.x major was de-risked by loading the pinned model's tokenizer and config against 5.14.1: it resolves cleanly, and 5.x provides `TokenizersBackend` natively — the `registry.py` monkey-patch that faked it is now a no-op. **Not verified:** the model *weights* load (1.2 GB + torch), so run the sidecar once before shipping.
    - The CI `pip-audit` job runs on a stock GitHub runner and is unaffected by the local crash.
- [~] **E13.S8: Container Image Scanning:** Trivy over all four images. Pin to digests, rebuild on base-image CVEs.
    - Trivy is wired into the S1 workflow (HIGH/CRITICAL, `ignore-unfixed`) for backend, frontend and docs, plus the mlrunner **base image** — mlrunner's own Dockerfile bakes torch and a 1.2 GB model into a layer, too slow to build per-PR, and its Python packages are covered by `pip-audit`. **Now actually executed**, and it was not a formality: it failed on first run and produced four real fixes (`x/crypto`, `nginx`, `python`, plus two Dockerfile bugs — see the run notes above). All four images now scan clean.
    - _Still partial:_ base images are pinned to **tags, not digests**, so a rebuild can silently pick up a different layer. `node:22-alpine` still carries HIGH/CRITICAL findings in its bundled npm tooling; it is a builder stage only and none of it reaches the shipped nginx image, so the gate does not see it — worth revisiting if the build stage ever ships.
- [x] **E13.S9: Go SAST Pass:** Run `gosec` + `staticcheck` across both modules and fix real findings.
    - Both modules clean at `-severity=medium`. Real fixes: the tenant API key disclosure, the loopback XSS, the `0755` config dir, and the `"claims"` context key (all detailed above), plus three staticcheck nits. Three `#nosec` annotations remain in `cli/internal/cmd/env.go`, each justified inline — `aigw run` executing the operator's own command and `--write` targeting the operator's own path are the features, not vulnerabilities.
- [~] **E13.S10: Automated Dependency Drift:** Enable Dependabot or Renovate for all six manifests (2 × `go.mod`, 3 × `package.json`, 1 × `requirements.txt`) plus Docker base images. Grouped weekly PRs, auto-merge on green for patch-level bumps.
    - _Partial — correction to the earlier "nothing started" note:_ Dependabot **is already running**, via GitHub security updates rather than a config file. `origin` carries open branches for `pgx`, `x/crypto`, `x/net`, `grpc`, `axios`, `torch` and `transformers` — the same bumps this pass made by hand, which is why several landed as duplicates.
    - _Missing:_ no `.github/dependabot.yml`, so there is no coverage of Docker base images (exactly the drift that produced the stale `nginx:1.27-alpine` findings), no grouping, no schedule control, and no auto-merge.
- [~] **E13.S11: Secret Scanning:** Enable GitHub push protection + a `gitleaks` CI job.
    - **⚠️ Currently DISABLED** (`if: false` in `security-scan.yml`), at the repo owner's request pending a gitleaks licence. **There is no secret scanning in CI while this is off.** Re-enable by deleting that one line — the job is fully configured and was verified passing.
    - _Note for whoever picks this up:_ a licence is **not** required. The job runs the gitleaks **OSS binary** (MIT), not `gitleaks/gitleaks-action` — the *action* is the part that demands a paid `GITLEAKS_LICENSE` on org-owned repos. Running the binary directly is free for organizations, so the job can be switched back on without buying anything.
    - The job scans with `fetch-depth: 0`, so it covers history rather than just the working tree.
    - **GitHub push protection is already enabled** — correcting two earlier notes in this file that said it still had to be turned on. It proved itself immediately: it rejected the very commit that added `.gitleaks.toml`, because the allowlist quoted Stripe-shaped fixture values verbatim.
    - Allowlisting is therefore **by fingerprint** (`commit:path:rule:line`) in `.gitleaksignore`, not by value in `.gitleaks.toml`. A value-anchored allowlist has to copy the string it is excusing, which turns the config into a file of key-shaped strings and gets the push blocked — listing a secret to declare it harmless is self-defeating. A fingerprint names one finding without reproducing it, and is *narrower*: scoped to a single occurrence rather than every match of that value.
    - Path-based allowlists were ruled out first, for a different reason: they mute an entire file. Verified by planting AWS/GCP/Stripe keys beside the placeholders and watching a path-scoped config swallow all of them. `condition = "AND"` is not honoured by gitleaks 8.30, so it cannot narrow one either.
    - **Two real credentials found in local git history** — see the escalation below. They are the reason this story is not closed.
    - _Still partial:_ the CI job itself is **disabled** (see above). Push protection is on and covers new pushes, but nothing scans history until the job is switched back on.

> ### ⚠️ Escalation: two credentials in local git history
>
> Found by the new gitleaks gate. Neither is reachable from `origin` or `public`, so **CI passes and nothing is exposed on GitHub** — but both are real and both are in the local clone's objects:
>
> | Commit | Value | Action |
> |---|---|---|
> | `307c8fa` *(feat: Implement SSO via OIDC)* | Google OAuth **client secret**, `backend/.env.example:23` | Rotate in Google Cloud Console |
> | `877f25e2` *(Add tenants and agents.md)* | `JWT_SECRET` value in `backend/.env.example:12` | Rotate; it signs admin session tokens |
>
> Neither value is in `HEAD` — the working tree has `<your-…>` placeholders. Deliberately **not** allowlisted: rotate first, then the finding disappears with the credential. Local history cleanup (`git gc` after dropping the unreferenced commits, plus the stash at `ca5aa1c`) is a separate step and was left to the repo owner rather than rewritten automatically.
>
> Note `backend/.env.example` also ships a real-looking `ADMIN_PASSWORD` default in `HEAD`. gitleaks does not flag it, but a committed default admin password is worth replacing with a placeholder.

**Definition of done:** `govulncheck` clean (no reachable findings) on both Go modules ✅, JS criticals/highs either patched or documented as unreachable ✅, mlrunner scanned ✅, all images scanned ⚠️ (wired, never executed — S8), and CI blocking regressions ✅.
