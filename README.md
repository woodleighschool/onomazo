# Onomazo

ὀνομάζω (_onomázō_) — “to name.”

---

Onomazo reconciles names across Microsoft Intune and Jamf Pro from one YAML policy. It can print a read-only plan, run once, or watch continuously.

The planner evaluates a complete inventory snapshot before any writes, so collisions are deterministic across providers. Rename requests pass through a local intent ledger: if an MDM still reports the old name after a request, Onomazo waits for the configured cooldown instead of submitting the same command every poll.

## Usage

Start with [`config.example.yaml`](config.example.yaml), then provide credentials through the referenced environment variables.

```bash
# Parse the configuration and compile every CEL expression.
onomazo validate --config config.yaml

# Apply optional overlays in order.
onomazo validate --config config.yaml --config site.yaml

# Fetch live inventory and print a read-only plan.
onomazo plan --config config.yaml
onomazo plan --config config.yaml --output json

# Apply one cycle or continue at reconcile.poll_interval.
onomazo run --config config.yaml --once
onomazo run --config config.yaml
```

`plan` never submits renames or updates rename state. Its JSON output includes the old-compatible comparison fields `device`, `platform`, `serial`, `user`, `to`, and `rule`, plus source, status, reason, and action.

## Configuration

Configuration files are merged in argument order. Mappings merge recursively, while scalars and lists replace earlier values. Environment substitution, strict decoding, defaulting, validation, and CEL compilation happen once after the merge. An environment placeholder must occupy a complete scalar, such as `${JAMF_CLIENT_SECRET}`; string interpolation is intentionally unsupported.

Naming expressions receive these typed values:

- `device`: `source`, `namespace`, `id`, `current_name`, `serial_number`, `platform`, `enrolled_at`, `user_id`, `model`, `os_version`, `last_seen_at`, and `attributes`.
- `user`: `present`, `id`, `mail_nickname`, `user_principal_name`, `display_name`, `department`, `groups`, and `attributes`.
- `vars`: string values produced by the configured variable cases.
- `slug(string)`: normalises a value for use in a device name.

Overrides are checked first. Variables collect every matching case and reject conflicting values. Naming rules then run in file order, with the first match winning. Fixed override names are authoritative and are never silently suffixed. Other collisions use deterministic ranks and sequence suffixes without truncating names.

## Reconciliation and state

Every cycle fetches current device names and primary-user references. Newly seen devices and changed primary users are enriched immediately. Stable device details and Entra identities are refreshed on their separate TTLs, allowing attribute, email, department, and group changes to flow through without re-fetching everything on every minute poll.

State defaults to memory and is fully functional for a long-running process. Configure file state when cooldowns must survive restarts:

```yaml
state:
    type: file
    path: /var/lib/onomazo/state.json
```

The JSON file is written atomically with mode `0600`, and an applying process holds an exclusive writer lock. A malformed configured state file fails closed. Changing a desired name supersedes an outstanding intent immediately; repeated identical requests wait for `rename_retry_after` and become stalled after `rename_max_attempts`.

## Provider APIs

Intune inventory uses Microsoft Graph v1.0. Renaming uses Graph's beta `setDeviceName` action because Microsoft does not expose that action in v1.0. Entra enrichment fetches only users referenced by the current inventory and checks configured groups transitively.

Jamf uses OAuth client credentials and the current JSON endpoints:

- computers: v4 bulk inventory and inventory-detail update;
- mobile devices: v2 bulk detail and update, which is the current endpoint family exposed for iOS inventory;
- mobile rename requests do not enable Jamf's persistent `enforceName` option.

Required privileges depend on configured features:

- Microsoft Graph: `DeviceManagementManagedDevices.Read.All`, `DeviceManagementManagedDevices.PrivilegedOperations.All`, `User.Read.All`, and `GroupMember.Read.All`.
- Jamf Pro API role: `Read Computers`, `Update Computers`, `Read Mobile Devices`, and `Update Mobile Devices`.

## Development

```bash
mise run build
mise run generate
mise run test
mise run lint
mise run fmt-check
```
