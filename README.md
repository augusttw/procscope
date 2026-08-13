# procscope

[Português (Brasil)](README.pt-BR.md)

`procscope` is a local observability CLI for Linux. It quickly shows what a process is consuming and which services it communicates with—without agents or a full observability stack.

The project uses only the Go standard library and `/proc`. Collection is hidden behind the `observe.Source` interface, so Netlink, eBPF, pprof, and OpenTelemetry support can be added without coupling those technologies to the CLI or analysis packages.

## Installation

Requires Linux and Go 1.22 or newer.

```sh
go install github.com/augusttw/procscope/cmd/procscope@latest
# Or, from the repository:
make build
```

## Quick start

```sh
# Run and monitor a command until it exits
procscope run -- ./my-server --port 8080

# Monitor a process; Ctrl-C stops only procscope
procscope attach 1234
procscope attach --once 1234
procscope attach --once --json 1234

# Inspect network activity owned by the process
procscope ports 1234
procscope connections 1234

# Record a 30-second JSON trace and diagnose it
procscope record --duration 30s --interval 500ms --output before.json 1234
procscope doctor --file before.json

# Compare snapshots or the final samples from two traces
procscope diff before.json after.json
```

Dependencies are inferred from common remote ports: PostgreSQL (`5432`), Redis (`6379`), and HTTP/HTTPS (`80`, `443`, `3000`, `8000`, `8080`, `8443`). This is a hint, not protocol inspection.

## Metrics and diagnostics

Each sample includes CPU usage since the previous sample, RSS, virtual memory, uptime, threads, file descriptors, I/O bytes, and TCP/TCP6 sockets. CPU usage is zero on the first sample because no previous measurement exists yet.

`doctor` applies transparent rules for high CPU usage, zombie or uninterruptible-sleep states, excessive file descriptors, RSS/FD growth, and large numbers of `TIME_WAIT` sockets. A single snapshot can only evaluate the current state; a trace also enables trend detection.

## Architecture

```text
cmd/procscope       entry point
internal/cli        commands and terminal experience
internal/observe    Source contract and sampler
internal/procfs     current Linux collector
internal/model      versioned snapshots and traces
internal/analysis   diff and doctor rules
internal/storage    JSON persistence
internal/format     terminal output
```

Current limitations: TCP/TCP6 only; kernel permissions may hide file descriptors owned by other users; dependency detection is port-based; `USER_HZ=100` is assumed, as on Linux kernels for commonly supported architectures.

## Development

```sh
make test
make vet
make build
```
