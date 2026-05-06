# Monitoring

## Start

```bash
cd monitoring
docker compose up -d --build
```

| Service | URL |
|---------|-----|
| Grafana | http://localhost:3005 |
| Prometheus | http://localhost:9093 |
| Prometheus-2 | http://localhost:9092 |

Grafana login: `admin` / `admin`

## Stop

```bash
docker compose down
```

## Add cluster nodes

Edit `prometheus/prometheus.yml` — one block per node:

```yaml
- targets:
    - <node-ip>:9701
  labels:
    node: <node-name>
```

## Access from another machine

```bash
ssh -L 3005:localhost:3005 \
    -L 9093:localhost:9093 \
    <user>@<groot1.niova.io>
```

Then open `http://localhost:3005`.
