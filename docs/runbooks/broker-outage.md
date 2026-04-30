# Runbook: Broker Outage

## Symptoms
- Messages queue up in `InMemoryQueue` but are not processed
- `fast_path_latency` metric spikes to 5s+
- `queue_length` metric grows without bound
- Telegram bot stops responding to spam

## Diagnosis
```bash
curl -s http://localhost:8080/api/metrics | jq '.counters'
# Look for: queue_length growing, processed_count flat
```

## Resolution

### In-Memory Queue (current default)
1. Restart the tg-spam process: `docker-compose restart tg-spam`
2. In-flight messages are lost — acceptable for non-critical moderation
3. Verify recovery: `curl http://localhost:8080/api/metrics | jq '.counters.spam_checks'`

### External Broker (future)
1. Check broker health: `redis-cli ping` or `nats-server --healthz`
2. If broker is down, fall back to in-memory: set `--queue.backend=memory`
3. Restore broker, drain backlog: `tg-spam queue-drain --from=dead-letter`
4. Verify no message loss via idempotency keys

## Prevention
- Monitor `queue_length` metric — alert if > 1000 for 5 minutes
- Enable `--retention.enabled` to auto-cleanup stale events
- Use external broker for production multi-tenant deployments

## Rollback
If new broker config causes issues:
```bash
# Revert to in-memory queue
tg-spam --queue.backend=memory
```

## Affected Components
- `app/moderation/queue.go` — InMemoryQueue
- `app/events/pipeline.go` — runQueueWorker
- `app/events/listener.go` — Do() event loop
