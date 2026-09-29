# Go/No-Go Release Checklist

## Pre-Deployment Verification

### Configuration
- [ ] Configuration file (`configs/config.yaml`) contains production DSN
- [ ] `server.internal_token` has been replaced with a strong random value (`openssl rand -hex 32`)
- [ ] `aisaas.internal_token` has been replaced with a strong random value
- [ ] JWT secret in config is a strong random value (minimum 32 characters)
- [ ] `database.dsn` uses a production MySQL instance (not dev/staging)
- [ ] `aisaas.base_url` points to production aisaas service

### Infrastructure
- [ ] MySQL 8.0+ database has been created and migrations applied
- [ ] Database credentials are stored securely (not in config file)
- [ ] Network connectivity to MySQL verified from server
- [ ] Network connectivity to aisaas service verified from server
- [ ] Firewall rules allow traffic on admin port (default 8081) and websocket port (default 8080)

### Observability
- [ ] Prometheus metrics endpoint accessible: `curl http://localhost:8081/metrics`
- [ ] Prometheus alert rules deployed: `deploy/alerts.yml`
- [ ] Grafana dashboard imported: `deploy/dashboards/dashboard.json`
- [ ] Alert routing configured (PagerDuty/Slack integration)
- [ ] Log aggregation configured (journald/ELK/CloudWatch)

### Testing
- [ ] Smoke tests pass: `go test ./test/smoke/... -count=1`
- [ ] Unit tests pass: `go test ./... -count=1`
- [ ] Integration tests pass: `go test -tags integration ./test/integration/... -count=1`
- [ ] Go build succeeds: `go build -tags silero ./...`
- [ ] Go vet passes: `go vet ./...`
- [ ] Go fmt check passes: `gofmt -l .` (no output expected)

### Rollback
- [ ] Previous version binary backed up
- [ ] Database rollback script prepared (if schema changes)
- [ ] Rollback procedure documented and tested
- [ ] On-call team notified of deployment window

## Deployment

### Execution
- [ ] Deployment started during low-traffic window (if applicable)
- [ ] Deployment command executed
- [ ] Service started: `systemctl restart xiaozhi-esp32-server-go`
- [ ] Service health verified: `curl http://localhost:8081/healthz`

### Post-Deployment
- [ ] Smoke tests pass in production environment
- [ ] WebSocket connections established successfully
- [ ] OTA endpoint responds correctly
- [ ] Metrics are being collected in Prometheus
- [ ] No error logs in journald: `journalctl -u xiaozhi-esp32-server-go --since "5 minutes ago" | grep -i error`

## Post-Release (1 hour after)

- [ ] Grafana dashboard shows normal metrics
- [ ] No increase in error rates
- [ ] Device connections stable
- [ ] No increase in latency percentiles
- [ ] On-call team confirms system healthy

## Rollback Decision Criteria

If ANY of these conditions are met, initiate rollback immediately:

1. More than 5% of devices disconnect within 5 minutes of upgrade
2. Error rate on any endpoint exceeds 1%
3. p99 latency exceeds 5 seconds
4. Memory usage exceeds 90% of limit
5. Process crash loop detected
6. Critical alert triggered (P1/P2 severity)

## Sign-Off

| Role | Name | Date | Signature |
|---|---|---|---|
| Deployer | | | |
| On-Call SRE | | | |
| Team Lead | | | |
