# Monitoring

Prometheus + Grafana for the Niova control plane.

## Prerequisite: a metrics-emitting control plane

The dashboards have no data until a control-plane **proxy** is running and
exposing Prometheus metrics on the scrape target (`localhost:9701` by default).
Requirements:

- The proxy binary must be built from code that includes `controlplane/proxy/metrics.go`
  (a stale build silently exposes no `/metrics` endpoint).
- The proxy must run with metrics enabled (`-metrics`, default on) on `-mp 9701`.
- If the proxy runs on another host/port, edit the scrape target in
  `prometheus/prometheus.yml` (see [Add cluster nodes](#add-cluster-nodes)).

Verify the target is being scraped before debugging dashboards:

```bash
curl -s http://localhost:9701/metrics | grep -v '^#' | head        # proxy emitting?
curl -s http://localhost:9093/api/v1/targets | grep -o '"health":"[^"]*"'  # want "up"
```

## Start

```bash
cd monitoring
docker compose up -d --build
```

| Service | URL |
|---------|-----|
| Grafana | http://localhost:3005 |
| Prometheus | http://localhost:9093 |

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
