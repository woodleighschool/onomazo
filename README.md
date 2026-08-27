# onomazo

[![Release](https://img.shields.io/github/v/release/woodleighschool/onomazo?display_name=tag&sort=semver)](https://github.com/woodleighschool/onomazo/releases/latest)
[![CI](https://github.com/woodleighschool/onomazo/actions/workflows/ci.yaml/badge.svg?branch=main)](https://github.com/woodleighschool/onomazo/actions/workflows/ci.yaml)
[![Go](https://img.shields.io/github/go-mod/go-version/woodleighschool/onomazo?logo=go)](https://github.com/woodleighschool/onomazo/blob/main/go.mod)
[![Container](https://img.shields.io/badge/container-ghcr.io-2496ED?logo=github&logoColor=white)](https://github.com/orgs/woodleighschool/packages/container/package/onomazo)
[![License](https://img.shields.io/github/license/woodleighschool/onomazo)](https://github.com/woodleighschool/onomazo/blob/main/LICENSE)

> **ὀνομάζω** (_onomázō_) — “to name”

Reconciles device names across Microsoft Intune and Jamf Pro from one YAML policy. It can run once from the command line or continuously as a service.

Planning uses a complete inventory snapshot and resolves collisions deterministically. Rename intentions are recorded so a slow MDM does not receive the same command every poll.

> [!WARNING]
> This project may be unstable or have bugs, use with caution.
> Also expect breaking changes between releases for now.

## 🚀 Usage

Download CLI archives for macOS, Linux, or Windows from the [latest release](https://github.com/woodleighschool/onomazo/releases/latest), or use the container `ghcr.io/woodleighschool/onomazo:rolling`.

Start with the example policy and an environment file for its `${...}` values:

```bash
cp config.example.yaml config.yaml
touch .env
```

Fill `.env` with values for the `${...}` names in `config.yaml`. The container commands below read this file; export the same values in your shell when using a downloaded binary.

| Command              | Behaviour                                    |
| -------------------- | -------------------------------------------- |
| `onomazo validate`   | Validate configuration and exit              |
| `onomazo plan`       | Print a read-only naming plan                |
| `onomazo run --once` | Apply one reconciliation cycle and exit      |
| `onomazo run`        | Apply immediately, then continue on interval |

If `config.yaml` is in the current directory, `--config` may be omitted. Multiple `--config` flags apply overlays in order.

### Run once

```bash
onomazo run --once
```

With the container:

```bash
docker run --rm \
  --env-file .env \
  --volume "$PWD/config.yaml:/config.yaml:ro" \
  ghcr.io/woodleighschool/onomazo:rolling \
  run --once
```

### Run continuously

```bash
onomazo run
```

The container runs continuously by default:

```bash
docker run --rm \
  --env-file .env \
  --volume "$PWD/config.yaml:/config.yaml:ro" \
  ghcr.io/woodleighschool/onomazo:rolling
```

Daemon mode writes structured JSON to stderr. Lifecycle and material reconciliation events use `info`, warnings and failures use `warn` or `error`, and successful cycle summaries plus routine no-op evaluations use `debug`.

## ⚙️ Configuration

Configuration is strict: unknown fields fail, lists replace earlier lists, and mappings merge recursively. Environment placeholders must occupy the whole value, such as `${JAMF_CLIENT_SECRET}`.

Runtime settings resolve from `ONOMAZO_*` environment variables, then the corresponding YAML value, then the default. CLI flags select configuration files or command behaviour rather than mirroring runtime settings.

| Environment variable                    | YAML fallback                   | Default  |
| --------------------------------------- | ------------------------------- | -------- |
| `ONOMAZO_LOG_LEVEL`                     | `log_level`                     | `info`   |
| `ONOMAZO_RECONCILE_POLL_INTERVAL`       | `reconcile.poll_interval`       | `1m`     |
| `ONOMAZO_RECONCILE_DEVICE_DETAILS_TTL`  | `reconcile.device_details_ttl`  | `1h`     |
| `ONOMAZO_RECONCILE_IDENTITY_TTL`        | `reconcile.identity_ttl`        | `15m`    |
| `ONOMAZO_RECONCILE_RENAME_RETRY_AFTER`  | `reconcile.rename_retry_after`  | `30m`    |
| `ONOMAZO_RECONCILE_RENAME_MAX_ATTEMPTS` | `reconcile.rename_max_attempts` | `3`      |
| `ONOMAZO_RECONCILE_CONCURRENCY`         | `reconcile.concurrency`         | `4`      |
| `ONOMAZO_STATE_TYPE`                    | `state.type`                    | `memory` |
| `ONOMAZO_STATE_PATH`                    | `state.path`                    | empty    |

| Section              | Purpose                                        |
| -------------------- | ---------------------------------------------- |
| `connections`        | Microsoft Graph and Jamf API credentials       |
| `devices`            | Intune and Jamf inventory sources              |
| `identity`           | Optional Entra user and group enrichment       |
| `naming.constraints` | Allowed names and maximum length               |
| `naming.overrides`   | Fixed names or exclusions for matching devices |
| `naming.variables`   | Reusable CEL-derived naming values             |
| `naming.rules`       | Ordered CEL naming rules                       |
| `naming.collisions`  | Deterministic ranking and suffixes             |
| `reconcile`          | Polling, refresh, retry, and cooldown timing   |
| `state`              | In-memory or file-backed rename intentions     |

Naming expressions receive typed `device`, `user`, and `vars` values plus `slug(string)`. Overrides run first, followed by naming rules in file order.

File state survives restarts and is written atomically:

```bash
ONOMAZO_STATE_TYPE=file
ONOMAZO_STATE_PATH=/var/lib/onomazo/state.json
```

## 🔄 Reconciliation

Each cycle refreshes device and primary-user data, enriches only what the policy needs, calculates the complete plan, then applies eligible renames. A changed desired name supersedes an older intention; identical requests wait for their configured retry window.

Intune uses Microsoft Graph for inventory and rename requests. Jamf uses OAuth client credentials and its current computer and mobile-device APIs. Required permissions depend on which providers and enrichment fields are enabled.

## 🧑‍💻 Development

```bash
mise run build
mise run generate
mise run test
mise run lint
mise run fmt-check
```

Tests use local servers and synthetic identities; provider credentials are not required.

## 📄 License

Licensed under the [Apache License 2.0](LICENSE).
