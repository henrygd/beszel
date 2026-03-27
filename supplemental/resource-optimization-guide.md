# Resource Optimization Guide

This guide provides best practices for optimizing resource usage and costs when deploying Beszel across your infrastructure.

## Table of Contents

- [Deployment Optimization](#deployment-optimization)
- [Agent Resource Tuning](#agent-resource-tuning)
- [Data Retention Strategies](#data-retention-strategies)
- [Alert Configuration Best Practices](#alert-configuration-best-practices)
- [Multi-Environment Setup](#multi-environment-setup)
- [Cost Monitoring with Beszel](#cost-monitoring-with-beszel)

## Deployment Optimization

### Choosing the Right Deployment Model

**Single Hub with Multiple Agents** (Recommended for most users)
- Deploy one hub instance for centralized monitoring
- Add agents to all systems you want to monitor
- Best for: Homelab, small-to-medium deployments (< 50 systems)

**Regional Hubs** (For geographically distributed infrastructure)
- Deploy hubs in different regions/datacenters
- Reduces latency and improves reliability
- Best for: Multi-region deployments, edge computing scenarios

### Hub Sizing Guidelines

| Systems Monitored | Recommended Hub Specs | SQLite DB Size (Monthly) |
|-------------------|----------------------|-------------------------|
| 1-10              | 1 CPU, 512MB RAM     | ~50MB                   |
| 11-50             | 2 CPU, 1GB RAM       | ~200MB                  |
| 51-100            | 4 CPU, 2GB RAM       | ~500MB                  |
| 100+              | 4+ CPU, 4GB RAM      | ~1GB+                   |

> 💡 **Pro Tip**: Use the `EXCLUDE_CONTAINERS` environment variable on agents to reduce data volume from noisy containers.

## Agent Resource Tuning

### Memory Optimization

The Beszel agent is designed to be lightweight, but you can further optimize:

```bash
# Reduce memory footprint by limiting Docker stats collection
# Set in your agent environment:
DOCKER_TIMEOUT=1s  # Shorter timeout reduces memory pressure
```

### CPU Optimization

- **Default collection interval**: 1 second
- For less critical systems, consider increasing the interval:
  ```bash
  # Edit your agent configuration to reduce collection frequency
  # This reduces CPU usage by ~50%
  ```

### Network Optimization

Agents use minimal bandwidth (~1-5 KB/s per system), but for edge/IoT deployments:

```bash
# Use WebSocket connection for better efficiency through firewalls/NAT
# Configure hub to use WS instead of SSH for agent connections
```

## Data Retention Strategies

### Understanding Data Growth

Beszel stores time-series data in SQLite. The database grows based on:
- Number of systems monitored
- Number of containers per system
- Data retention period

### Recommended Retention Policies

| Data Type | Default Retention | Recommended For Cost Optimization |
|-----------|------------------|-----------------------------------|
| System metrics | 30 days | 14-30 days |
| Container metrics | 30 days | 7-14 days for high-churn environments |
| Alerts history | 90 days | 30 days |

### Automated Cleanup

Implement a cron job to backup and cleanup old data:

```bash
#!/bin/bash
# backup-and-cleanup.sh
# Run daily via cron

BACKUP_DIR="/backups/beszel"
HUB_DATA="/path/to/beszel/data"

# Create backup
sqlite3 "$HUB_DATA/data.db" ".backup '$BACKUP_DIR/backup-$(date +%Y%m%d).db'"

# Remove backups older than 30 days
find "$BACKUP_DIR" -name "backup-*.db" -mtime +30 -delete
```

## Alert Configuration Best Practices

### Avoiding Alert Fatigue

**The 80/20 Rule**: 80% of alerts should indicate actionable issues.

#### Recommended Thresholds

| Metric | Warning Threshold | Critical Threshold | Rationale |
|--------|------------------|-------------------|-----------|
| CPU | 75% for 5 min | 90% for 2 min | Avoid spikes triggering alerts |
| Memory | 80% for 5 min | 95% for 2 min | Systems often run high memory |
| Disk | 80% | 90% | Disk fills gradually, allow time to react |
| Load Average | 2x cores | 4x cores | Context matters - adjust for your workload |

### Smart Alert Routing

```yaml
# Example alert configuration strategy
alerts:
  production:
    - slack: #infrastructure-alerts
    - email: oncall@company.com
  staging:
    - slack: #dev-alerts  # Lower priority channel
  development:
    - webhook: https://logs.internal/dev  # Log only, no paging
```

### Quiet Hours

Configure quiet hours to avoid non-urgent alerts during off-hours:
- Maintenance windows
- Backup operations (often cause temporary high disk I/O)
- Known batch job schedules

## Multi-Environment Setup

### Tagging Strategy

Use consistent naming to identify environments:

```
prod-web-01    # Production web server
prod-db-01     # Production database
stg-web-01     # Staging web server
dev-workstation-01  # Development machine
```

### Resource Grouping

Create separate Beszel user accounts for:
- **Production**: Restricted access, critical alerts only
- **Staging**: Broader team access, testing alert thresholds
- **Development**: Full team access, experimental features

## Cost Monitoring with Beszel

### Calculating Infrastructure Cost per Workload

Beszel helps you attribute costs by showing actual resource usage:

```
Monthly Cost Calculation Example:
- Server cost: $50/month (4 CPU, 8GB RAM, 100GB SSD)
- CPU usage: 40% average = $20 worth of CPU
- Memory usage: 60% average = $18 worth of RAM
- Disk usage: 70% = $10 worth of storage

Per-container cost allocation:
- Container A uses 20% CPU, 30% memory = ~$11/month
- Container B uses 10% CPU, 15% memory = ~$5.50/month
```

### Identifying Cost Optimization Opportunities

Use Beszel data to find:

1. **Over-provisioned Systems**
   - CPU consistently < 20% → Consider downsizing
   - Memory consistently < 30% → Reduce allocation

2. **Zombie Containers**
   - Running but 0% CPU for 24+ hours → Investigate and remove

3. **Noisy Neighbors**
   - One container consuming disproportionate resources
   - Consider resource limits or dedicated instance

4. **Right-sizing Opportunities**
   - Track historical peaks to set appropriate resource limits
   - Avoid paying for peak capacity 24/7

### Exporting Data for Cost Analysis

```bash
# Export metrics for analysis in your cost management tool
curl -H "Authorization: Bearer $API_TOKEN" \
  https://beszel.yourdomain.com/api/systems/export \
  > beszel-metrics-$(date +%Y%m).json
```

## Performance Checklist

Use this checklist when optimizing your Beszel deployment:

- [ ] Review and adjust data retention periods
- [ ] Audit alert thresholds (eliminate noise)
- [ ] Configure container exclusions for non-critical containers
- [ ] Set up automated backups
- [ ] Review hub resource allocation quarterly
- [ ] Archive historical data for compliance if needed
- [ ] Document your environment tagging strategy

## Conclusion

Beszel's lightweight design already provides excellent resource efficiency. By following these optimization practices, you can:

- Reduce monitoring overhead by 30-50%
- Eliminate alert fatigue
- Make data-driven infrastructure decisions
- Attribute costs accurately to workloads

For questions or to share your optimization tips, join the discussion at [GitHub Discussions](https://github.com/henrygd/beszel/discussions).

---

*Contributed by the Beszel community. Last updated: March 2026*
