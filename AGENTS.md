# AGENTS.md

Repository guidance for Onomazo.

## Approach

- Stay within the requested scope and preserve unrelated local changes.
- This is a purpose-built reconciliation tool, not a SaaS platform. Prefer direct code for current provider behavior.
- Simplify and modernize existing code before adding abstractions, compatibility layers, migrations, or aliases.
- Follow the shared Woodstar tooling baseline while keeping Onomazo's behavior authoritative.

## Repository Map

- CLI and runtime loops: `cmd/onomazo`
- Composition and reconciliation: `internal/app`
- Strict configuration and generated schema: `internal/config`
- Typed CEL expressions: `internal/expression`
- Provider-neutral values: `internal/domain`
- Deterministic naming and collisions: `internal/planner`
- Rename intentions and persistence: `internal/state`
- Remote clients: `internal/provider/{microsoft,jamf}`

Keep provider transport out of the planner and naming policy out of provider packages.

## Commands

Use Mise tasks as the repository contract.

- Dependencies: `mise run deps`
- Build: `mise run build`
- Tests: `mise run test`
- Lint: `mise run lint`; fixes: `mise run lint-fix`
- Format: `mise run format`; check: `mise run fmt-check`
- Generated schema: `mise run generate`
- Module and workflow checks: `mise run tidy-check`, `mise run workflow-lint`

Ordinary tests require no provider credentials or live services. For release-path changes, also check the Docker build and `.goreleaser.yaml`.

## Reconciliation Rules

- `plan` is read-only. It mustn't mutate providers, rename intentions, or locks.
- Build a complete provider snapshot before planning. Collision decisions must be deterministic across configured sources.
- Device identity is source, provider resource namespace, and provider ID; serial protects persisted state when an ID is reused.
- Never silently truncate a desired name. Invalid names and exhausted suffixes remain visible outcomes.
- Persist a rename intention before submitting its request. Respect cooldowns and clear intentions only when the desired name is observed.
- State writes remain atomic, mode `0600`, malformed-state fail-closed, and single-writer locked.
- Continuous cycles run one at a time and propagate cancellation into provider requests.

## Engineering Rules

- Configuration decoding stays strict. Environment placeholders occupy a complete scalar and resolved secrets are never logged.
- CEL inputs remain typed and must be updated across domain values, declarations, provider mappings, and tests.
- Prefer concrete Go types, small consumer-owned interfaces, and explicit wrapped errors.
- Provider tests use local HTTP servers. Keep real identities, group IDs, serials, credentials, and local naming policy out of code and fixtures.

## Commits

- Use focused Conventional Commits.
- Don't push, publish, release, or contact live providers unless explicitly requested.
- Report checks run, skipped checks, live-provider proof boundaries, and unresolved failures.
