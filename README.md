# onomazo

> **ὀνομάζω** (_onomázō_) — “to name”

Reconciles device names across Microsoft Intune and Jamf Pro from one YAML policy. It can run once from the command line or continuously as a service.

Planning uses a complete inventory snapshot and resolves collisions deterministically. Rename intentions are recorded so a slow MDM does not receive the same command every poll.

> [!WARNING]
> This project may be unstable or have bugs, use with caution.
> Also expect breaking changes between releases for now.

## 🚀 Usage

Download an archive from the [latest release](https://github.com/woodleighschool/onomazo/releases/latest), or build it with Mise. Start from [`config.example.yaml`](config.example.yaml). If `config.yaml` is present in the current directory, `--config` may be omitted:

```bash
onomazo validate
onomazo plan
onomazo plan --output json
onomazo run --once
onomazo run
```

Multiple `--config` flags apply overlays in order. `plan` is always read-only.

The published container runs the continuous service by default:

```bash
docker run --rm \
  --volume "$PWD/config.yaml:/config.yaml:ro" \
  ghcr.io/woodleighschool/onomazo:rolling \
  run --config /config.yaml
```

Daemon mode writes structured JSON to stderr. Lifecycle and material reconciliation events use `info`, warnings and failures use `warn` or `error`, and successful cycle summaries plus routine no-op evaluations use `debug`.

## ⚙️ Configuration

Configuration is strict: unknown fields fail, lists replace earlier lists, and mappings merge recursively. Environment placeholders must occupy the whole value, such as `${JAMF_CLIENT_SECRET}`.

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

```yaml
state:
  type: file
  path: /var/lib/onomazo/state.json
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
