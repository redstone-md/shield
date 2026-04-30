# Telegram API Degradation

## Symptoms
- High `gateway_check_latency` in `/api/metrics`
- Telegram updates stop arriving (no `spam_checks` counter increment)
- `procEvents` errors in logs

## Diagnosis
1. Check metrics: `curl localhost:8080/api/metrics | jq .histograms.gateway_check_latency`
2. Verify bot token: `curl "https://api.telegram.org/bot<TOKEN>/getMe"`
3. Check update stream: look for `[DEBUG] start listening for updates` in logs

## Mitigation
1. Bot API returns 429 (rate limit): reduce `--idle-duration`, wait for reset
2. Bot API returns 5xx: enable exponential backoff, check @telegramapilog
3. Bot token revoked: update token in config, restart service
4. Network connectivity: verify DNS (`api.telegram.org`), check firewall

## Recovery
1. Service auto-reconnects via `GetUpdatesChan` — no manual action needed for transient failures
2. For prolonged outages (>5min): restart the service. Pending queue processes after reconnection
3. After recovery: verify `spam_checks` counter resumes incrementing

## Prevention
- Monitor `gateway_check_latency` histogram — p99 > 2s indicates degradation
- Set alert on `spam_checks` counter dropping to 0 for >3 intervals
