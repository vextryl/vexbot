# VexBot dashboard development roadmap

This is an implementation roadmap, not authorization to change production.
Each phase is small enough for a fresh Codex session to execute from this
document and the repository. Passing a phase permits a commit, never a deploy,
restart, or production configuration change.

## Dependency map

```text
0 decisions/ADRs
       |
1 contracts and seams ----+---- 2 PostgreSQL/migrations
       |                  |             |
       +------------------+---- 3 shared config service/Discord bridge
                                        |
                                     4 HTTP API/runtime
                                    /              \\
                         5 OAuth/security          6 dashboard build/UI
                                    \\              /
                                     7 integrated verification
                                                |
                                     8 approved rollout
```

Phases 1 and 2 may start in parallel after phase 0 if they use the approved
configuration contract. Phase 5 and the static/UI-shell portion of phase 6 may
run in parallel after phase 4. Their integration, phase 7, and phase 8 are
sequential.

## Phase 0 — Resolve product and operations decisions

**Depends on:** none. **Parallel work:** none; this unblocks all implementation.

Decide and record the first configuration fields/defaults; legacy environment
fallback and cutover; `DISCORD_GUILD_ID` future; snowflake storage type;
PostgreSQL hosting/backups/network/TLS; migration runner; public HTTPS origin
and proxy owner; OAuth client/redirect URI; authorization policy/session TTLs;
frontend build tool; and database test approach. Update
`docs/architecture.md` or add short ADRs for material choices.

**Acceptance criteria**

- Each v1 setting has validation, default, owner, and migration behavior.
- A named operator and safe migration, backup, and restore procedure exist.
- OAuth, proxy, and permission choices are concrete enough for runtime config.

**Testing:** tabletop failure/restore and authorization scenarios are reviewed.

**Review/commit checkpoint:** architecture/ADR review, then one `docs:` commit.
Do not implement unresolved placeholders.

## Phase 1 — Define contracts and isolate configuration seams

**Depends on:** phase 0. **Can run with:** phase 2 schema planning.

Introduce transport-neutral configuration types, validation/defaulting,
repository interfaces, and service inputs/outputs. Specify the v1 API resource
and error-envelope contract in tests or code-adjacent docs. Extract only config
decisions currently passed directly into Discord commands; do not move voice,
audio, recording, or transcription logic. Define legacy global-environment
defaults during the transition.

**Acceptance criteria**

- Shared types contain no HTTP, SQL, `disgo`, OAuth-token, or request types.
- Revision/conflict behavior is explicit.
- Existing slash-command behavior is unchanged under legacy configuration.
- The contract identifies auth/authz inputs without implementing OAuth.

**Testing:** behavior-focused Go unit tests with fake stores cover valid,
invalid, defaulted, missing, and revision-conflict configurations.

**Review/commit checkpoint:** service-boundary review, then focused `refactor:`
or `feat:` commit; run `go test ./...`, `go vet ./...`, and gofmt.

## Phase 2 — Add PostgreSQL configuration persistence and migrations

**Depends on:** phase 0 database/migration choices and phase 1's frozen
contract. **Can run with:** phase 1 only until that contract is frozen.

Add forward-only numbered SQL migrations and a PostgreSQL `guild_config`
repository. Include only approved typed fields, canonical IDs, revisioning,
timestamps, and DB constraints. Provide the approved explicit migration command
or job; never auto-migrate destructively at bot startup. Leave recording
artifacts and live session state on existing storage.

**Acceptance criteria**

- A clean database migrates under advisory lock.
- Reads/upserts preserve defaults and reject invalid data; competing revisions
  return the documented conflict.
- Queries honor cancellation and do not expose DB details to services.
- No token, credential, recording, or transcript enters this schema.

**Testing:** PostgreSQL integration tests on an ephemeral database cover empty
DB migration, CRUD, constraints, conflicts, timestamps, and connection/context
failure. Add a dedicated CI database job; do not weaken the current
network-isolated Go checks.

**Review/commit checkpoint:** schema/migration and restore-plan review, then a
focused `feat:` commit. Run all Go gates, DB tests, and `git diff --check`.

## Phase 3 — Wire the shared service through Discord safely

**Depends on:** phases 1 and 2. **Sequential:** prove the service before HTTP.

Compose repository/service in `internal/app`; inject it into Discord command
handling instead of duplicated command-boundary settings. Retain legacy env
defaults until phase 0's cutover plan says otherwise. Keep `session.Manager`
authoritative for active status and preserve filesystem recording behavior.

**Acceptance criteria**

- Discord and service tests show identical validation/defaulting behavior.
- `/join`, `/stop`, `/status`, retention, and transcription retain behavior for
  the current configured guild.
- DB startup/shutdown and failure behavior match readiness policy; logs contain
  no secrets.

**Testing:** focused command/service-adapter tests plus full Go/vet/gofmt/Docker
gates.

**Review/commit checkpoint:** command-regression review and one `refactor:` or
`feat:` commit.

## Phase 4 — Add HTTP runtime and API foundation

**Depends on:** phase 3. **Unblocks:** phases 5 and 6.

Add a `net/http` server composed by `internal/app`, explicit listener config,
timeouts, request-size limits, orderly shutdown, and phase-0 `/healthz` and
`/readyz` semantics. Add `/api/v1/` routing, JSON content types, stable errors,
and dependency-injected handlers. Do not accidentally publish configuration
mutation before authentication is ready.

**Acceptance criteria**

- API routes can never fall through to the future SPA handler.
- Handler tests prove status, content type, safe errors, and cancellation.
- Shutdown stops HTTP intake before DB closure and preserves existing
  gateway-first recording finalization.
- Health/readiness are documented and testable without a public deployment.

**Testing:** `httptest` route/error tests with fakes, app lifecycle tests, and
existing full Go/Docker gates.

**Review/commit checkpoint:** HTTP contract/lifecycle review and one `feat:`
commit.

## Phase 5 — Implement Discord OAuth, sessions, and authorization

**Depends on:** phase 4 and phase 0 security choices. **Can run with:** phase
6 static/UI-shell work; authenticated integration waits for both.

Implement server-side authorization-code login/callback/logout, short-lived
single-use state, opaque persistent sessions, cookie rotation, CSRF protection,
permission predicates, and per-guild server-side authorization. Validate OAuth,
database, external URL, and session-key runtime config without logging values.
Retain refresh tokens only if phase 0 approved their necessity.

**Acceptance criteria**

- Replayed/expired/wrong/missing state, wrong issuer, exchange failure, and
  untrusted Origin are safely rejected.
- Protected requests verify session and server-side target-guild authorization;
  cross-guild access fails.
- Cookies have approved Secure/HttpOnly/SameSite/path/lifetime attributes;
  login rotates and logout invalidates them.
- Responses/logs expose no token, code, secret, or session ID.

**Testing:** state/cookie/CSRF/predicate unit tests; `httptest` callback and
endpoint tests; PostgreSQL expiry tests; mocked browser OAuth. Require security
review before merge.

**Review/commit checkpoint:** security review, focused `feat:` commit, all
Go/database/static checks.

## Phase 6 — Build and embed the dashboard

**Depends on:** phase 4 API contract. **Can run with:** phase 5 until protected
flows integrate. **Must finish after phase 5** before full system verification.

Create standalone `web/` with reproducible build to `web/dist`. Implement only
the approved dashboard: login/logout, authorized guild selection, config
read/edit with conflict feedback, and loading/error/inaccessible states. Embed
output through a dedicated Go asset handler. Use relative API paths and no
client secrets. Hash-cache assets and exclude API, auth, health, and readiness
routes from SPA fallback.

**Acceptance criteria**

- UI builds/tests without Go imports or a real Discord account.
- Final image serves index/assets/client routes; no API is masked by fallback.
- The same UI can later deploy separately with no API contract change.
- Accessibility, validation, failure, and concurrency behavior match the API.

**Testing:** UI component/type/lint/build tests; Go asset-handler tests; browser
tests against mocked OAuth/API; Docker runtime asset smoke test.

**Review/commit checkpoint:** UI/API review and focused `feat:` commit. Apply a
consistent generated-artifact policy for the selected toolchain.

## Phase 7 — Integrated verification and operational readiness

**Depends on:** phases 5 and 6. **Sequential:** validates the combined system.

In an isolated environment: migrate an empty DB, start bot/API, complete mocked
OAuth, authorize a guild, update config, prove the Discord path observes it,
exercise session expiry/logout/CSRF failure, and graceful shutdown. Extend CI
without removing existing Go/Docker checks to cover DB, UI, API contract, and
browser flow.

**Acceptance criteria**

- Suites pass reproducibly from a clean checkout.
- Migration backup/restore is rehearsed; readiness responds correctly to
  required dependency failure.
- Logs are reviewed for secret leakage and useful audit context.
- Documentation/privacy/operator configuration are updated wherever OAuth or
  runtime behavior makes current text inaccurate.

**Testing:** unit, integration, browser, image, migration/restore, and security
regression suites. Leave a concise operator test record in the PR.

**Review/commit checkpoint:** release-readiness review and appropriate `test:`,
`docs:`, or `ci:` commit. Do not deploy.

## Phase 8 — Separately approved staging and production rollout

**Depends on:** phase 7 and explicit user approval. **Sequential.**

Current deployment has no public HTTPS proxy, PostgreSQL service, port exposure,
health verification, or automatic rollback. Add those only in an approved
operational change. Validate staging HTTPS/OAuth first, then use the approved
migration/backup procedure. Production requires a verified target revision,
preserved recordings/Whisper mounts, health checks, login/config roundtrip, log
review, and a reversible image/database recovery plan.

**Acceptance criteria:** staging and production evidence satisfies those checks
and the explicit approval scope. **Review/commit checkpoint:** code review is
separate from deployment approval; never bundle unreviewed production action
into a feature commit.
