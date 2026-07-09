# Runbook: Provider Outage (OpenAI/Gemini)

## Symptoms
- Slow-path checks return errors consistently
- Breaker trips on all providers
- Logs: `breaker "..." is open, dropping request`

## Diagnosis
```bash
# Check breaker status via metrics
curl -s http://localhost:8080/api/metrics | jq '.counters'

# Check recent provider errors
grep -c "provider.*error" /var/log/shield/app.log
```

## Recovery Steps
1. **Verify provider status** — check OpenAI/Gemini status pages
2. **Breaker auto-recovers** — default `HalfOpenDelay` is 30s
3. **If persistent** — increase `FailuresToTrip` in config to tolerate transient errors
4. **Fallback** — fast-path (regex, stopwords, samples) continues without LLM
5. **Budget exhaustion** — check `budget_remaining` in metrics; increase `MaxRequestsPerHour` if needed

## Prevention
- Configure multiple providers for redundancy
- Set appropriate `FailuresToTrip` and recovery delays
- Monitor `slow_path_errors` metric; alert at >10/min
