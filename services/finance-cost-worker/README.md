# finance-cost-worker

Worker service for the Phase C calc engine. Consumes chunk messages from
RabbitMQ (published by `finance-cost-orchestrator`), executes the calc batch
against PostgreSQL, and publishes a "chunk-done" event back to the orchestrator.

## Status

**Bootstrap (S8c.6)** — service skeleton only. Wires config + zerolog +
Prometheus `/metrics` + `/healthz` + RabbitMQ connection + worker_id
auto-generation + an empty `Worker.Run` loop + signal-driven graceful shutdown.
Real consumer / calc executor / publisher logic lands in S8c.7.

## Ports

| Purpose          | Port |
|------------------|------|
| Metrics / health | 8093 |

## Worker ID

Each worker instance gets a unique `worker_id` used in logs and chunk locks. If
`worker.worker_id` (or env `WORKER_ID`) is empty, main generates one as
`<hostname>-<pid>`.

## Run locally

```bash
make run
# or via binary
make build && ./bin/finance-cost-worker
```

`/metrics` and `/healthz` are served on port 8093.

## Configuration

See `config.yaml`. Smaller DB pool than the orchestrator (`max_open=6`,
`max_idle=2`) since each worker pod runs calc batches sequentially.
