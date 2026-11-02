# VexBot dashboard architecture

## Purpose and baseline

This is an incremental dashboard architecture. It preserves existing recording
and transcription behavior until a phase explicitly changes it.

VexBot is currently one Go process: `internal/app` composes a Discord gateway,
an in-memory `session.Manager`, and filesystem-backed recordings/transcripts.
Process-wide environment values include one `DISCORD_GUILD_ID`, used for
development guild command registration. There is no HTTP listener, database,
frontend, or OAuth implementation.

The target is a versioned Go HTTP API, transport-neutral application services,
PostgreSQL-backed guild configuration, and an embedded dashboard over HTTPS.
The UI remains logically independent so its compiled assets can later be hosted
separately without changing the API or client code.

## Component model

```text
Browser dashboard (web/)                 Discord slash-command adapter
  relative /api/v1 requests                         |
             |                                      |
             +----------- HTTPS / HTTP ------------+
                                      |
                           HTTP/OAuth adapters
                   API errors, cookies, CSRF, routing
                                      |
                         application services
             config validation, authorization inputs, use cases
                         /                    \\
             repository interfaces         live session/status ports
                    |                              |
          PostgreSQL adapters                 existing Discord/session/
     guild config, OAuth state, sessions        recording components

Filesystem recordings, WAV files, timelines, and transcripts remain local
artifacts; PostgreSQL is not their source of truth in the initial dashboard.
```

`internal/app` remains the composition root. It creates runtime configuration,
PostgreSQL connections, services, Discord adapters, the HTTP server, and their
ordered shutdown. `main.go` stays thin.

| Area | Responsibility | Must not own |
| --- | --- | --- |
| `internal/app` | Wire dependencies; start/stop Discord, HTTP, and database components. | Business rules or SQL. |
| Shared application package | Guild-config reads/updates, validation/defaulting, use-case authorization inputs. | HTTP, `disgo`, SQL, and OAuth types. |
| PostgreSQL adapter | Migrations plus guild config, OAuth state, and dashboard sessions. | Recording files, live session state, authorization policy. |
| Discord adapters | Translate Discord events/commands to application inputs. | Direct SQL or duplicate config rules. |
| HTTP API adapter | `/api/v1/` routing, JSON, limits/timeouts, stable errors, auth middleware. | UI-specific workflows or persistence logic. |
| OAuth/session adapter | Discord code flow and server-side session/CSRF lifecycle. | Trusting browser-supplied guild authorization. |
| `web/` dashboard | UI presentation; documented relative API calls. | Go imports, direct DB access, bot/OAuth secrets. |
| Asset handler | Initially serve embedded built UI and SPA fallback. | Rewriting `/api/*` to the SPA. |

Exact application package and interface names are deferred to implementation.
They must remain narrow and use-case-owned. Discord and HTTP call the same
configuration service contract.

## API and service design

The API begins at `/api/v1/`. Path versioning is required from the first public
endpoint; incompatible changes get a new version. UI calls use relative paths,
for example `/api/v1/guilds/{guild_id}/config`.

Handlers return JSON with a stable safe error envelope, e.g.
`{"code":"forbidden","message":"You cannot manage this guild."}`. The HTTP
adapter maps validation, unauthenticated, forbidden, missing, conflict, and
unexpected failures to 400, 401, 403, 404, 409, and 500. Underlying errors are
logged only server-side.

Services accept a verified actor and target guild rather than raw OAuth tokens,
HTTP requests, or Discord SDK values. The existing in-memory `session.Manager`
remains authoritative for live status initially; persisting transient recording
state would add recovery semantics that are not yet required.

## Persistence

PostgreSQL owns durable dashboard data. The first schema is intentionally
narrow:

- `guild_config`: canonical decimal-string `guild_id` primary key, only
  approved typed overrides, revision for optimistic concurrency, and timestamps.
- Separate expiring tables for OAuth login transactions and opaque dashboard
  sessions.

Store Discord snowflakes as canonical decimal text unless the team explicitly
accepts the signed-`BIGINT` range constraint. Do not use a catch-all JSON config
document or store bot credentials in guild config. Database checks enforce
positive limits such as retention count and upload size.

Filesystem recording directories, their current global retention behavior, WAV
files, timelines, transcripts, and active transcription remain outside
PostgreSQL. Guild-scoped retention is a separate product/storage decision.

Migrations are committed, numbered, forward-only SQL. The proposed operating
model is an explicit migration command/job protected by a PostgreSQL advisory
lock and timeout. The bot must not silently apply a destructive migration at
startup. Recovery is forward repair or tested restore, not assumed down SQL.

## Dashboard delivery

Place the UI in `web/` with its own manifest, lockfile, source, tests, and
reproducible build. Its integration artifact is `web/dist/`. A dedicated asset
package embeds that output with `//go:embed`, serves hashed assets with
immutable caching, and serves the SPA entry point only for non-API client
routes.

Initially UI and API share an HTTPS origin. This simplifies secure cookies,
OAuth callbacks, and CORS. Later, the same `web/dist` may be served by a CDN or
web server while `/api` routes to Go; no client secret, Go import, or API
contract change is allowed. Cross-origin UI hosting is deferred because it
changes cookie, CSRF, and CORS design.

TLS terminates at a reverse proxy. Go trusts forwarding headers only with
explicit trusted-proxy configuration. Bare HTTP is not a public dashboard
endpoint. Planned health endpoints distinguish liveness (`/healthz`) and
dependency readiness (`/readyz`).

## Discord OAuth and security

Login uses Discord's server-side OAuth 2.0 authorization-code flow with minimum
`identify` and `guilds` scopes, not implicit OAuth. State is cryptographically
random, short-lived, single-use, and bound to a pre-auth session. The callback
validates state, expiry, exact configured redirect URI, and Discord's returned
issuer before server-side code exchange. Client secret, codes, access/refresh
tokens, cookies, and bot tokens are never rendered, logged, or returned.

Use an opaque server-side dashboard session ID in a host-only `Secure`,
`HttpOnly`, `SameSite=Lax` cookie with explicit path/lifetime. Rotate at login,
expire on idle and absolute lifetimes, and delete on logout. Store OAuth state
separately and consume it once. Unsafe cookie-authenticated requests also need
a CSRF token/header or strict Origin validation; SameSite is not sufficient.

Every guild request is authorized server-side by both:

1. The bot recognizes the guild as installed/managed.
2. The authenticated user passes the selected guild-management policy.

The recommended first policy is Discord `Administrator` or `Manage Guild`.
Never trust a callback guild hint, browser-provided guild ID, or stale client
authorization alone. Add rate limits for login/callback/mutations, request size
limits, timeouts, safe audit logs, and CSP/HSTS/`X-Content-Type-Options`/
`Referrer-Policy` headers. Authentication responses are `no-store`.

## Decisions and tradeoffs

| Decision | Why | Tradeoff |
| --- | --- | --- |
| PostgreSQL owns config/sessions, not recordings | Preserves the local recording model and limits scope. | Recording search/history is not DB-backed initially. |
| Shared Discord/HTTP services | One validation/defaulting behavior. | Requires careful extraction from handlers. |
| Path-versioned JSON API | Stable seam for independent UI deployment. | Contract maintenance/tests are required. |
| Embedded same-origin SPA first | One deployable unit and simple cookies. | UI cadence initially follows the bot image. |
| Server-side OAuth/opaque sessions | Limits token exposure and supports expiry/revocation. | Adds session persistence/key operations. |
| Explicit migrations | Auditable, recoverable database changes. | Adds a deployment step and backup discipline. |

## Unresolved decisions

Do not silently invent these requirements. Record the decisions in an
implementation PR or short ADR before dependent work proceeds.

1. The exact first guild-config fields, defaults, override behavior, and
   whether each replaces a global environment value. Whisper/bot-global options
   must not be assumed per-guild.
2. `DISCORD_GUILD_ID`'s future: development-only registration, optional value,
   or a multi-guild registration model.
3. PostgreSQL hosting/network/TLS/backups/restore owner, migration tool/operator,
   and integration-test database mechanism.
4. Public HTTPS origin, reverse-proxy/certificate owner, trusted proxy range,
   OAuth redirect URI, and readiness behavior when Discord or PostgreSQL fails.
5. Permission verification/revalidation policy, session TTLs, key rotation, and
   whether OAuth refresh tokens are genuinely needed.
6. Frontend framework/toolchain, CSP details, accessibility/browser support,
   and audit-history requirements beyond safe logs.
