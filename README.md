# Onomazo

A Go service that reconciles managed-device names from rules defined in a YAML configuration file. It can run once for scheduled jobs or continuously.

## Features

- **YAML-based naming rules** – all behaviour is defined in the config file.
- **Microsoft Graph support** – uses the `setDeviceName` action on managed devices.
- **Ordered rule engine** – rules evaluate in priority order using Go templates.
- **User and group enrichment** – device data includes primary user attributes and group memberships.
- **Metadata overlays** – attach additional attributes to devices using match conditions.
- **Duplicate name handling** – global or per-rule policies (append suffix, skip, error, overwrite).

## Configuration

The service reads a single YAML file (`config.yaml` by default).
See `config.example.yaml` for a full reference.

# Environment Variables

Settings can be provided via environment variables or CLI arguments:

| Variable          | Description                                                 |
| ----------------- | ----------------------------------------------------------- |
| `TENANT_ID`       | Azure AD tenant ID (required)                               |
| `CLIENT_ID`       | App client ID (required)                                    |
| `CLIENT_SECRET`   | App client secret (required)                                |
| `GRAPH_BASE_URL`  | Graph API base (default `https://graph.microsoft.com/v1.0`) |
| `POLL_INTERVAL`   | Sync interval for continuous mode (default `5m`)            |
| `LOG_LEVEL`       | `debug`, `info`, `warn`, `error` (default `info`)           |
| `DRY_RUN`         | `true` = no renames                                         |
| `MAX_NAME_LENGTH` | Global name length cap (default `63`)                       |

## Duplicate Policy

`settings.duplicatePolicy.scope` determines how name uniqueness is checked:

- **global** – unique across the entire tenant
- **per-user** – unique only among devices for the same user
- **per-platform** – unique within each OS platform

Rules may override the global settings on a per-rule basis.

## Metadata Overlays

- Each entry has a **priority**; higher entries override lower ones when keys collide.
- All matching entries are merged.
- `match.anyGroup` and `match.allGroup` use Entra group IDs to test membership.
- Other match keys compare directly to device/user attributes.

`staticNames` allow fixed names with optional `enforce: true` to ensure they aren’t changed elsewhere.

## Matchers

Matchers exist under `metadata[].match` and `rules[].match`.

### Group matchers

- `anyGroup` — match if the user is in _any_ listed groups
- `allGroup` — match only if in _all_ listed groups

### Attribute matchers

- Any `key: value` pair is compared against `.Attr "key"`.
- `platforms` is shorthand for matching the OS.
- Strings match case-insensitively unless prefixed with `regex:` or written as `/pattern/`.

Attributes available include:

- Intune device fields (serial, platform, category, etc.)
- Primary user fields (username, department, mail nickname, group IDs)
- Metadata values merged from overlays

**Permissions required:**
`DeviceManagementManagedDevices.Read.All`
`DeviceManagementManagedDevices.PrivilegedOperations.All`
`User.Read.All`
`Group.Read.All`

## Templates

Rule templates use Go `text/template`.

### Helpers

- `upper`, `lower`, `title`
- `default value fallback`
- `replace old new`
- `substr start len`
- `truncate len`
- `clean` — trims, uppercases, removes invalid characters, normalises separators

Example:

```yaml
template: '{{ clean (printf "%s-%s" (.Attr "campus") (.Attr "username")) }}'
```

If a template references an attribute that isn’t available, the rule is skipped and evaluation continues with the next rule.

Rules run in descending `priority`.
A rule stops further evaluation unless `stopProcessing: false`.

## Running

Set the required environment variables, then run the binary:

```bash
export TENANT_ID="00000000-0000-0000-0000-000000000000"
export CLIENT_ID="11111111-1111-1111-1111-111111111111"
export CLIENT_SECRET="replace-me"

# Build
go build ./...

# Run once
./onomazo -config config.yaml -once

# Continuous loop
./onomazo -config config.yaml
```

Use `-once` for scheduled execution.
Use `-config` to point to a non-default file.
