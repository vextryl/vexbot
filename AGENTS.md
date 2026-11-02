# VexBot development guide

## Established repository conventions

- The Go module is `github.com/vextryl/vexbot`; `main.go` is thin and `internal/app` is the composition root.
- Organize production code by focused domain/integration packages under `internal/`; keep Discord-specific code under `internal/discord` and avoid leaking it into core recording/transcription packages.
- Keep tests beside the code they cover as `*_test.go`, use the same package where practical, and retain the existing table-free, behavior-focused test style.
- CI runs `go test ./...`, `go vet ./...`, formatting checks, and Docker builds. Deployment builds and tests an image, then replaces the single VexHub container over SSH.
- Existing commit history uses Conventional Commit prefixes such as `feat:`, `fix:`, `refactor:`, `docs:`, `ci:`, `build:`, and `style:`.

## Required working preferences

- Use idiomatic Go names; prefer lowercase, singular filenames where practical. Create small, focused packages and abstractions only where they simplify a real boundary. Avoid speculative layers and unnecessary complexity.
- For ordinary local execution, use `go run .` when its native dependencies are available. Run `go test ./...` for test changes; also run relevant focused tests while iterating.
- Preserve current behavior when changing architecture. Make behavior changes explicit, covered by tests, and update affected configuration or operational documentation.
- Use Conventional Commit messages if a commit is approved.

## Approval and production safety

- Never commit, push, create releases, deploy, restart, stop, or replace production services without explicit user approval for that action.
- Treat Discord tokens, SSH keys, environment files, recordings, and Whisper assets as sensitive. Do not add secrets to the repository, image, logs, or documentation examples.
- Before any approved production change, verify the target revision and configuration, keep persistent recordings/mounts intact, and validate health/logs after rollout. Prefer a reversible, documented path.

## Architecture documentation

- Keep `README.md` and deployment/configuration instructions accurate whenever package boundaries, runtime dependencies, environment variables, Docker behavior, CI/CD, or operator workflows change.
- Record material architectural decisions near the affected documentation (or add a short ADR when warranted), including the problem, decision, and operational consequences.
