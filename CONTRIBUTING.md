# Contributing to AI Gateway

Thanks for your interest in contributing! This guide covers how to set up your
environment, the conventions we follow, and how to get a change merged.

By participating you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

---

## Ways to Contribute

- **Report bugs** — open an issue using the bug template.
- **Suggest features** — open an issue using the feature template.
- **Improve docs** — fixes to `docs/` and code comments are always welcome.
- **Submit code** — pick an open issue (look for `good first issue`) or discuss
  a proposal first for larger changes.

> **Security issues are different.** Never open a public issue for a
> vulnerability — follow [SECURITY.md](SECURITY.md).

---

## Project Layout

| Component | Path | Stack |
|---|---|---|
| Backend (proxy + admin) | `backend/` | Go |
| CLI (`aigw`) | `cli/` | Go |
| Admin UI | `frontend/` | React 19 / Vite / TypeScript / pnpm |
| Docs site | `docs/` | pnpm |
| Landing site | `landing/` | pnpm |

Architecture details live in [`AGENTS.md`](AGENTS.md) and `backend/CLAUDE.md`.

---

## Development Setup

### Prerequisites

- Go 1.22+
- Docker + Docker Compose
- Node.js 20+ and pnpm

### Backend

```bash
cd backend
cp .env.example .env       # edit values; set JWT_SECRET + ADMIN_PASSWORD
make tidy                  # fetch deps
make infra                 # postgres + redis + otel + jaeger
make dev                   # run both servers (loads .env)
```

| Command | Action |
|---|---|
| `make dev` | run proxy (:8080) + admin (:8081) locally |
| `make build` | compile to `bin/gateway` |
| `make test` | `go test -race -count=1 ./...` |
| `make test-cover` | coverage report |
| `make lint` | `golangci-lint run ./...` |
| `make tidy` | `go mod tidy` |
| `make up` / `make down` | full Docker stack up/down |

### CLI

```bash
cd cli
make build                 # → bin/aigw
make test
```

### Frontend / docs

```bash
cd frontend                # or docs / landing
pnpm install
pnpm dev                   # frontend dev server on :5173
pnpm build
```

---

## Coding Conventions

### Go

- Format with `gofmt` + `goimports` before committing.
- Use `log/slog` JSON for all logging.
- Check errors with `errors.Is` / `errors.As`; wrap with context:
  `fmt.Errorf("doing X: %w", err)`.
- HTTP routing via `github.com/go-chi/chi/v5`; cross-cutting concerns
  (auth, tenant context) go in middleware.
- Use `golang.org/x/sync/errgroup` for goroutine groups.
- **TDD** — write the failing test first. Always run with `-race`. Tests live
  alongside source as `_test.go`.
- **No new abstractions** unless there are 3+ concrete uses.

### Frontend

- React Query for all server state.
- Use `cn()` from `src/lib/utils.ts` for class merging.
- Match existing page aesthetics (monospace font, existing color tokens).

---

## Commit & Branch Conventions

- Branch from `master`: `feature/<short-name>`, `fix/<short-name>`,
  `docs/<short-name>`.
- We use **[Conventional Commits](https://www.conventionalcommits.org/)**:
  - `feat: add rate limiting middleware`
  - `fix: correct token expiry comparison`
  - `docs: document upstream env vars`
  - `test:`, `refactor:`, `chore:` as appropriate.
- Keep commits focused; squash noise before opening the PR.

---

## Pull Request Process

1. Fork and create your branch from `master`.
2. Make your change with tests. Ensure:
   - `make test` passes (backend), `pnpm build` succeeds (frontend/docs).
   - `make lint` is clean.
   - No secrets, `.env` files, or generated artifacts are committed.
3. Update docs (`docs/`, `README.md`, env tables) if behavior changes.
4. Open the PR using the template; link the issue it resolves.
5. A maintainer reviews. Address feedback by pushing follow-up commits.
6. Once approved and CI is green, a maintainer merges.

### Developer Certificate of Origin

By submitting a PR you certify that you wrote the code or have the right to
submit it under the project's [AGPL-3.0 license](LICENSE). Sign off your
commits with `git commit -s` (adds a `Signed-off-by` line).

---

## Questions

Open a [Discussion](../../discussions) or an issue. We're happy to help you get
started.
