# xiaozhi-esp32-server-go Runbook

## 1. Deployment Architecture

### Overview
`xiaozhi-esp32-server-go` is a Go WebSocket server for ESP32 device communication. It handles device connections, audio streaming, and dialogue orchestration.

### Components
- **WebSocket Server** (`:8080`): Device connections, audio streaming, protocol handling
- **Admin HTTP Server** (`:8081`): Health checks, metrics, OTA, internal API
- **EventBus**: Internal event pub/sub for device lifecycle events
- **Dialogue Orchestrator**: Audio pipeline, STT/LLM/TTS via aisaas service
- **Memory Store**: Per-device conversation history (window memory)
- **Device Store**: Device registry and metadata

### Dependencies
- MySQL 8.0+ for device and message persistence
- aisaas service (ykt-aisaas) for AI services (STT, LLM, TTS)
- Prometheus for metrics collection
- Grafana for visualization

### Environment Variables
| Variable | Description | Example |
|---|---|---|
| `SERVER_ADMIN_TOKEN` | Admin API authentication token | `openssl rand -hex 32` |
| `AISAAS_INTERNAL_TOKEN` | Internal token for aisaas service calls | `openssl rand -hex 32` |
| `DB_DSN` | MySQL connection string | `user:pass@tcp(host:3306)/db?parseTime=true` |

### Startup Command
```bash
cd /opt/xiaozhi-esp32-server-go
./xiaozhi-esp32-server-go
```

---

## 2. Day-to-Day Operations

### Starting the Server
```bash
# Production
systemctl start xiaozhi-esp32-server-go

# Or direct
./xiaozhi-esp32-server-go
```

### Stopping the Server
```bash
# Graceful shutdown (30s drain)
systemctl stop xiaozhi-esp32-server-go

# Force kill
pkill -f xiaozhi-esp32-server-go
```

### Restarting
```bash
systemctl restart xiaozhi-esp32-server-go
```

### Checking Status
```bash
# Systemd status
systemctl status xiaozhi-esp32-server-go

# Process running?
ps aux | grep xiaozhi-esp32-server-go

# Ports listening?
ss -tlnp | grep -E '8080|8081'
```

### Logs
```bash
# Journald logs
journalctl -u xiaozhi-esp32-server-go -f

# Last 100 lines
journalctl -u xiaozhi-esp32-server-go -n 100

# Since specific time
journalctl -u xiaozhi-esp32-server-go --since "1 hour ago"
```

---

## 3. Troubleshooting

### Issue: Server Won't Start

**Symptoms**: Process exits immediately or fails to bind ports.

**Diagnosis**:
```bash
# Check port conflicts
ss -tlnp | grep -E '8080|8081'

# Check config file exists and is valid
cat /opt/xiaozhi-esp32-server-go/configs/config.yaml

# Check environment variables
env | grep -E 'SERVER_|AISAAS_|DB_'
```

**Fix**:
- Ensure ports 8080 and 8081 are available
- Verify config.yaml syntax: `yamllint configs/config.yaml`
- Verify MySQL is reachable: `mysql -h <host> -u <user> -p -e "SELECT 1"`

### Issue: Devices Can't Connect

**Symptoms**: Devices failing WebSocket upgrade with "connection refused" or "invalid token".

**Diagnosis**:
```bash
# Check WebSocket server is listening
curl -i http://localhost:8081/healthz

# Check logs for connection errors
journalctl -u xiaozhi-esp32-server-go | grep -i error

# Verify aisaas service is reachable
curl -s http://ykt-aisaas:8190/healthz
```

**Fix**:
- Ensure firewall allows connections to port 8080
- Verify device tokens are valid in MySQL
- Check aisaas service is running and accessible

### Issue: High Memory Usage

**Symptoms**: `xiaozhi-esp32-server-go` RSS > 500MB or growing over time.

**Diagnosis**:
```bash
# Check memory usage
ps aux | grep xiaozhi-esp32-server-go

# Check metrics for memory trends
curl -s http://localhost:8081/metrics | grep process_resident

# Check goroutine count
curl -s http://localhost:8081/metrics | grep go_goroutines
```

**Fix**:
- Restart if memory leak suspected: `systemctl restart xiaozhi-esp32-server-go`
- Check for goroutine leaks in logs
- Consider memory limits in systemd unit

### Issue: Audio Quality Problems

**Symptoms**: Devices reporting audio glitches, TTS missing, STT inaccurate.

**Diagnosis**:
```bash
# Check audio buffer overflow metrics
curl -s http://localhost:8081/metrics | grep audio_buffer

# Check aisaas latency
curl -s http://localhost:8081/metrics | grep aisaas_request_duration
```

**Fix**:
- Check network latency to aisaas service
- Verify opus codec configuration in config.yaml
- Check VAD (voice activity detection) settings

### Issue: OTA Updates Failing

**Symptoms**: Devices not receiving firmware updates.

**Diagnosis**:
```bash
# Check OTA handler logs
journalctl -u xiaozhi-esp32-server-go | grep -i ota

# Verify firmware version configured
grep -i firmware configs/config.yaml

# Check device last_seen timestamp
mysql -e "SELECT device_id, firmware_version, last_seen_at FROM xiaozhi_device LIMIT 10;"
```

**Fix**:
- Update `latest_firmware_version` in config.yaml
- Ensure aisaas firmware endpoint is accessible
- Check device activation status in MySQL

---

## 4. Monitoring Metrics

### Key Prometheus Metrics

| Metric | Description | Alert Threshold |
|---|---|---|
| `xiaozhi_ws_connections` | Current WebSocket connections | > 160 (80% of max) |
| `xiaozhi_ws_connection_errors_total` | Connection error rate | > 10/min |
| `xiaozhi_aisaas_calls_total{status="error"}` | Aisaas error rate | > 5/min |
| `xiaozhi_aisaas_request_duration_seconds` | Aisaas API latency | p99 > 3s |
| `xiaozhi_db_errors_total` | Database error rate | > 1/min |
| `xiaozhi_http_request_duration_seconds` | HTTP request latency | p99 > 3s |
| `xiaozhi_audio_buffer_overflows_total` | Audio buffer overflow rate | > 0 |

### Dashboard
Grafana dashboard available at: `deploy/dashboards/dashboard.json`

Import with:
```bash
GRAFANA_TOKEN=<token> ./deploy/dashboards/import-dashboard.sh
```

### Alerting
Prometheus alert rules available at: `deploy/alerts.yml`

---

## 5. Scaling

### Horizontal Scaling (Multiple Instances)
- Each instance is stateless (uses MySQL for state)
- Load balancer needed for WebSocket connections (sticky sessions recommended)
- Each instance needs unique metrics port if running on same host

### Vertical Scaling
- Recommended: 2 vCPU, 4GB RAM minimum
- For > 500 concurrent devices: 4 vCPU, 8GB RAM
- Monitor goroutine count: should stay < 1000 for normal operation

### Resource Tuning
```yaml
# config.yaml
server:
  max_connections: 200  # Increase for more devices
  read_timeout: "30s"
  write_timeout: "30s"

database:
  max_open_conns: 50
  max_idle_conns: 10
```

---

## 6. Backup & Recovery

### Database Backup
```bash
# Full backup
mysqldump -h <host> -u <user> -p xiaozhi > backup_$(date +%Y%m%d).sql

# Incremental (binlog)
mysql -e "PURGE BINARY LOGS BEFORE NOW();"
```

### Restore
```bash
mysql -h <host> -u <user> -p xiaozhi < backup_20240929.sql
```

### Disaster Recovery
1. Stop service: `systemctl stop xiaozhi-esp32-server-go`
2. Restore DB: `mysql < backup.sql`
3. Start service: `systemctl start xiaozhi-esp32-server-go`
4. Verify: `curl http://localhost:8081/healthz`

---

## 7. Contacts

### On-Call Rotation
| Time | Primary | Secondary |
|---|---|---|
| Weekdays (9am-6pm) | @platform-team | @sre-team |
| Nights/Weekends | @oncall-sre | @oncall-platform |

### Escalation
1. Primary on-call
2. Secondary on-call
3. Team lead: @team-lead
4. Engineering manager: @eng-manager

### External Contacts
- **aisaas support**: aisaas-support@company.com
- **Hardware vendor**: hw-support@vendor.com
