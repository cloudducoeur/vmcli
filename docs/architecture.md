# Architecture

The MVP uses manual constructor wiring and four focused layers:

- `cmd`: Cobra commands and dependency wiring.
- `internal/config`: YAML loading and environment/path resolution.
- `internal/api`: HTTP client, Prometheus query response, metrics parser, and HTTP errors.
- `internal/cluster`: concurrent node checks and aggregation.
- `internal/tui`: Bubble Tea model and terminal rendering.

The TUI receives `VictoriaMetricsClient`; it never imports or calls `net/http` directly. Every node check has a context timeout and a failed node becomes `DOWN` without stopping the cluster view.
