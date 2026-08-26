# VictoriaMetrics APIs

The MVP uses the official HTTP surfaces:

- `GET /health` for liveness.
- `GET /metrics` for Prometheus exposition metrics.
- `GET /select/<projectID>/prometheus/api/v1/query` for instant PromQL queries.
- `GET /api/v1/rules` for vmalert rules (`vmcli rules`).
- `GET /api/v1/alerts` for vmalert alerts (`vmcli alerts`).
- `GET /api/v1/notifiers` for vmalert notifiers (`vmcli notifiers`).

The client preserves HTTP status failures as `HTTPError` values and supports
Basic Auth, bearer token authentication, custom CA certificates, and explicit
insecure TLS configuration.

`vmcli query` supports both instant and range queries:

- instant: `query`
- range: `query_range` via `--last` or `--from/--to`, with optional `--step`

The query TUI renders an `asciigraph` chart, supports multi-series overlay or
single-series focus, and auto-refreshes based on `--refresh` (or
`refreshInterval` when omitted).
