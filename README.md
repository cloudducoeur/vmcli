# vmcli

`vmcli` is a terminal supervisor for VictoriaMetrics Cluster. The initial MVP reads configured nodes, checks `/metrics`, runs PromQL queries, and provides a keyboard-driven overview.

## Installation

Prerequisite: Go `1.24+`.

From a local checkout:

```sh
cd vmcli
go install .
```

This installs `vmcli` into `$(go env GOPATH)/bin` (or `GOBIN` if set).

Alternative (local binary):

```sh
make build
./bin/vmcli version
```

## Quick start

```sh
go build -o vmcli .
vmcli tui --config ~/.config/vmcli/config.yaml --cluster production
vmcli nodes --output json
vmcli query 'up'
vmcli query --raw --output json 'up'
vmcli rules --output table
vmcli rules --group node_alerts
vmcli rules --name NodeExporterDown
vmcli rules --name NodeExporterDown --raw
vmcli alerts --output table
vmcli notifiers --output table
```

Configuration is loaded from `VMCLI_CONFIG` or `~/.config/vmcli/config.yaml`.

## Configuration

Create the default file with:

```sh
mkdir -p ~/.config/vmcli
cp config.example.yaml ~/.config/vmcli/config.yaml
```

Example:

```yaml
refreshInterval: 5s

clusters:
  production:
    vminsert:
      - https://vminsert01.example.org:8480
      - https://vminsert02.example.org:8480
    vmselect:
      - https://vmselect01.example.org:8481
      - https://vmselect02.example.org:8481
    vmstorage:
      - https://vmstorage01.example.org:8482
      - https://vmstorage02.example.org:8482
    vmalert:
      - https://vmalert.example.org
    tenant:
      accountID: 0
      projectID: 0
    auth:
      username: admin
      password: change-me
      # bearerToken: replace-basic-auth-with-a-token
    tls:
      caFile: /path/to/ca.pem
      insecureSkipVerify: false
```

`vminsert`, `vmselect`, and `vmstorage` contain the HTTP endpoints of the
configured VictoriaMetrics nodes. The first configured cluster is selected
when `--cluster` is omitted:

```sh
vmcli tui --cluster production
vmcli nodes --cluster production
vmcli query --cluster production 'up'
vmcli rules --cluster production
vmcli rules --cluster production --group node_alerts
vmcli rules --cluster production --name NodeExporterDown
vmcli rules --cluster production --name NodeExporterDown --raw
vmcli alerts --cluster production
vmcli notifiers --cluster production
```

`vmcli query` opens a graph-focused TUI by default (rendered with
`asciigraph`), with colored per-series charts and visible scales. Use `--raw`
to print query results to stdout
(and combine with `--output json|yaml|table`).
The query TUI includes a live chart and auto-refresh:

```sh
vmcli query --refresh 2s 'up'
vmcli query --last 5m --step 10s 'up'
vmcli query --from '2026-08-26T09:00:00Z' --to '2026-08-26T10:00:00Z' 'up'
```

If `--refresh` is omitted, it uses `refreshInterval` from config. Use
`--refresh 0s` to disable auto-refresh and refresh manually with `r`.
In query TUI: `s` toggles all-series vs single-series view, and `←/→` (or
`h/l`) selects the active series.
In `--raw` mode, refresh only runs when `--refresh` is explicitly provided.
Time range options:

- `--last 5m`: rolling window over the last duration
- `--from ... --to ...`: fixed interval
- `--step`: resolution for range queries (auto-derived if omitted)

Configuration lookup order is:

1. `--config /path/to/config.yaml`
2. `VMCLI_CONFIG=/path/to/config.yaml`
3. `~/.config/vmcli/config.yaml`

Use either `username` and `password` for Basic Auth or `bearerToken` for
Bearer authentication. Credentials are sent only by the HTTP client and are
never displayed in the TUI. Keep the configuration file readable only by its
owner when it contains credentials:

```sh
chmod 600 ~/.config/vmcli/config.yaml
```

Use `tls.caFile` for a private certificate authority on HTTPS endpoints.
When all configured endpoints use `http://`, the `tls` section is ignored.
Keep `insecureSkipVerify: false` in production; when set to `true`, CA file
loading is skipped and TLS certificate verification is disabled.

## Status

Implemented: YAML configuration, HTTP health/metrics client, concurrent node checks, authentication/TLS configuration, overview and node details TUI, PromQL query, vmalert rules/alerts/notifiers commands, query TUI chart with configurable auto-refresh, styled table output via `lipgloss`/`bubbles`, JSON/YAML output, tests.

Planned: labels and series, dashboard charts, query history, structured logging, automatic discovery, CI release workflow.
