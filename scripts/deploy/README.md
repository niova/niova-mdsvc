# ⚠️ Niova CTLPlane Deployment Guide

> **WARNING**
> These scripts are designed for test environments.
> Validate all paths and configurations before running in production.

---

## 🧩 Prerequisites

1. **Passwordless SSH** must be configured between nodes (required for `multinode` deployment).
2. **Serf** and **jq** must be installed on all nodes if using gossip-based discovery.
3. The **NFS volume** (if used) must be mounted consistently across all participating nodes.

---

## 🗂️ Configuration

Discovery and deployment parameters are defined in `config.yaml`.

### `config.yaml` Structure

```yaml
nodes:
  - 192.168.1.10
  - 192.168.1.11
  - 192.168.1.12

gossip:
  port_range:
    start: 10010
    end: 10020

ports:
  peer: 10000
  client: 10105

output_dir: /var/niova/cp
bin_dir: /usr/local/bin/niova
lib_dir: /usr/local/lib
```

* **nodes**: List of physical node IPs or hostnames.
* **gossip.port_range**: Range of ports for Serf gossip agents.
* **ports**: Base ports for Raft peering and client communication.
* **output_dir**: Base directory for generated configs, logs, and databases.
* **bin_dir**: Location of Niova binaries (`CTLPlane_pmdbServer`, `CTLPlane_proxy`).
* **lib_dir**: Location of required shared libraries.

---

## 🚀 Deployment

The `deploy.sh` script automates the generation of Raft configurations and process management.

### Usage

```bash
sudo ./deploy.sh [-m init|restart] [-t localhost|multinode] [-p port] <config.yaml>
```

### Options

* **`-m <mode>`**:
  - `init`: Clean install. Deletes existing configs/DBs and generates new UUIDs.
  - `restart`: Preserves existing data. Re-applies library paths and restarts services.
* **`-t <type>`**:
  - `localhost`: Runs all processes on the current machine (useful for dev).
  - `multinode`: Distributes processes across nodes listed in `config.yaml`. Requires SSH access and shared `output_dir` (NFS).
* **`-p <port>`**: Proxy metrics port passed as `-mp` to `CTLPlane_proxy`. Defaults to `9701`.

### Example

```bash
# Initial cluster setup on multiple nodes
sudo ./deploy.sh -m init -t multinode config.yaml
```

---

## 📡 Serf Gossip Interface

The `serf_gossip_interface.sh` script is used to discover the active Control Plane proxy via the gossip network.

### Usage

```bash
./serf_gossip_interface.sh <gossip_nodes_path> <gossip_key>
```

* **gossip_nodes_path**: Path to the generated `gossipNodes` file (found in `${output_dir}/config/`).
* **gossip_key**: The Serf encryption key (must match the one used in `deploy.sh`).

The script outputs the `PROXY_IP` and `PROXY_HTTP_PORT` of a healthy proxy node.

---

## 🛑 Management Commands

If `pdsh` is installed, you can manage the cluster across all nodes:

### Verification
```bash
pdsh -w <node_list> 'ps -ef | grep CTLPlane'
```

### Stopping Services
```bash
pdsh -w <node_list> 'sudo killall CTLPlane_pmdbServer CTLPlane_proxy'
```

---

## 🗄️ Artifact Locations

All generated files are stored in the `output_dir` specified in `config.yaml`:

* `config/`: RAFT, peer, and gossip configuration files.
* `log/`: Process-specific logs and stdout redirects.
* `db/`: RAFT state databases (`.raftdb`).