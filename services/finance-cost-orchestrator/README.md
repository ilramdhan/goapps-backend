# finance-cost-orchestrator

Coordinator service for the Phase C calc engine. Plans calc jobs into chunks,
publishes them to worker queues via RabbitMQ, consumes chunk-done events, and
finalizes the job once all chunks complete.

## Status

**Bootstrap (S8c.1)** — service skeleton only. Wires config + zerolog +
Prometheus `/metrics` + `/healthz` + RabbitMQ connection + an empty
`Coordinator.Run` loop + signal-driven graceful shutdown. Real planner /
publisher / coordinator state-machine logic lands in S8c.2 - S8c.5.

## Ports

| Purpose          | Port |
|------------------|------|
| Metrics / health | 8092 |

## Run locally

```bash
make run
# or via binary
make build && ./bin/finance-cost-orchestrator
```

`/metrics` and `/healthz` are served on the same port (8092).

## Configuration

See `config.yaml` for defaults. Secrets via env vars (`DATABASE_PASSWORD`,
`RABBITMQ_URL`, etc.). Standard viper precedence: env > file > default.
