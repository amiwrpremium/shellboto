# Monitoring

shellboto ships no metrics / Prometheus endpoint / OTEL
integration. What you watch is journald + filesystem + process
state.

## What to alert on

### 1. Service not running

```bash
systemctl is-active shellboto
```

Exit 0 = active. Anything else = wire to your pager.

### 2. Doctor failing

```bash
shellboto doctor || page
```

See [doctor.md](doctor.md).

### 3. Audit chain broken

```bash
shellboto audit verify || page
```

Schedule every 6 hours minimum. See
[../audit/hash-chain.md](../audit/hash-chain.md).

### 4. DB growing faster than expected

```bash
ls -la /var/lib/shellboto/state.db
```

Baseline your deployment's growth. Alert on > 2× weekly delta.

Common causes:

- auth_reject spam (attacker). Check with
  `shellboto audit search --kind auth_reject --since 24h | wc -l`
  — if > a few dozen, lock down.
- Heavy workload genuinely (more users, larger outputs).
- Retention too long.

### 5. Disk pressure

```bash
df -h /var/lib/shellboto
```

Generic OS-level monitoring. Alert at 80%, page at 95%.

## Log-based alerts

### ERROR level events

```bash
sudo journalctl -u shellboto --priority=err --since "5 minutes ago"
```

Any output → alert. Shouldn't fire in steady state.

### Unusual `auth_reject` spikes

```bash
sudo journalctl -u shellboto --output=cat --since "1 hour ago" \
    | jq -c 'select(.msg == "audit" and .kind == "auth_reject")' \
    | wc -l
```

Compare against baseline. Spike = probe.

### `role_changed` during off-hours

A 3 AM `role_changed` should page you. Someone's promoting or
demoting when you're not expecting it.

### `danger_confirmed` outside expected users

```bash
sudo journalctl -u shellboto --output=cat --since "1 day ago" \
    | jq -c 'select(.msg == "audit" and .kind == "danger_confirmed")'
```

Spot-check. Each of these was an admin actively tapping ✅ Run
— expected. Unexpected user ID = investigate.

## What to NOT alert on

- `shell_reaped` — routine.
- `shell_reset` — user action; fine.
- `shell_spawn` — routine.
- `command_run` with exit 0 — the happy path.

## Tooling wiring

### Prometheus (via blackbox exporter)

```yaml
# blackbox.yml
modules:
  shellboto_doctor:
    prober: script
    script:
      command: /usr/local/bin/shellboto
      args: [doctor]
      timeout: 30
```

Then alert on the `probe_success` metric.

### Nagios / Icinga NRPE

```
# /etc/nagios/nrpe.d/shellboto.cfg
command[shellboto_doctor]=/usr/local/bin/shellboto doctor
command[shellboto_audit_verify]=/usr/local/bin/shellboto audit verify
command[shellboto_service]=systemctl is-active shellboto
```

### Datadog / New Relic / PagerDuty

Scripted check via cron → write a file with OK/FAIL → your agent
picks it up. Or a journald filter + alert.

### Simple email-only

```
# /etc/cron.d/shellboto-alerts
# Check doctor + service + chain every 15 min; email on failure.
*/15 * * * * root \
    /usr/local/bin/shellboto doctor > /dev/null 2>&1 \
    && /usr/local/bin/shellboto audit verify > /dev/null 2>&1 \
    && systemctl is-active --quiet shellboto \
    || echo "shellboto check failed on $(hostname)" | mail -s "URGENT" you@you.net
```

## Dashboards

No native dashboard. If you ship logs to Loki / ELK:

Useful queries:

- Rate of `kind=command_run` over time (throughput).
- Count of `kind=auth_reject` by `user_id` (attacker activity).
- Distribution of `exit_code` across `kind=command_run`
  (error rate).
- Distribution of `duration_ms` for long-running commands
  (workload).
- Count of `kind=role_changed` over time (governance activity).

Ship + build the dashboard; shellboto is agnostic.

## SLOs (if you're serious)

A solo-operator bot doesn't need SLOs. For a team deployment:

- **Availability:** service `active (running)` 99% of the time
  (accepts ~7h downtime/month).
- **Latency:** a `/status` reply within 2 seconds, 99% of
  requests.
- **Audit integrity:** chain verify OK, 100% of scheduled runs.

Measure via your log-ingestion. shellboto itself is too small to
warrant heavyweight SLO tooling.

## Read next

- [../runbooks/](../runbooks/) — what to do when an alert fires.
- [logs.md](logs.md) — more on journald.
