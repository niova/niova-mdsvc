#!/bin/bash
# start_pumice.sh
# Starts CTLPlane proxy and server with execution logging.

set -e

log() {
    echo "[start_pumice][$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

CFG_FILE="$(readlink -f "$1")"
[ -f "$CFG_FILE" ] || { echo "Config file not found"; exit 1; }

log "Using config file: $CFG_FILE"

# ---- Parse YAML ----

BASE_DIR=$(awk -F: '$1=="base_dir" {gsub(/^[ \t]+|[ \t]+$/, "", $2); print $2}' "$CFG_FILE")
[ -n "$BASE_DIR" ] || exit 1

CONFIGS_DIR="${BASE_DIR}/configs"
LIB_DIR="${BASE_DIR}/lib"
LIBEXEC_DIR="${BASE_DIR}/libexec/niova"
CTL_DIR="${BASE_DIR}/ctl"
LOG_DIR="${BASE_DIR}/logs"

log "BASE_DIR=${BASE_DIR}"
log "CONFIGS_DIR=${CONFIGS_DIR}"

[ -d "${CONFIGS_DIR}" ] || { log "configs dir missing"; exit 1; }
[ -f "${CONFIGS_DIR}/gossipNodes" ] || { log "gossipNodes missing"; exit 1; }

# ---- Environment ----

export NIOVA_LOCAL_CTL_SVC_DIR="${CONFIGS_DIR}"
export LD_LIBRARY_PATH="/lib:${LIB_DIR}"
export NIOVA_INOTIFY_BASE_PATH="${CTL_DIR}"
export NIOVA_APPLY_HANDLER_VERSION=0
export USER_ENCRYPTION_KEY="81gavMyXh9dEMT7kM7gR+gS79ovzPwyjWmV1VA/TUII"
export AUTH_ENABLED="${AUTH_ENABLED:-true}"

log "Environment variables exported"

# Collect all IPs (space-separated → newline)
IPS=$(hostname -I | tr ' ' '\n')

log "Detected nodes IPs: $(echo "$IPS" | tr '\n' ' ')"

# ---- Raft UUID ----

RAFT_FILE=$(ls -1 "${CONFIGS_DIR}"/*.raft | head -n1)
[ -n "$RAFT_FILE" ] || { log "No .raft file found"; exit 1; }

RAFT_UUID=$(basename "$RAFT_FILE" .raft)
log "Using RAFT_UUID=${RAFT_UUID}"

# ---- Peer UUID ----
# Match if ANY detected IP appears in any .peer file

PEER_FILE=$(
  for ip in $IPS; do
    grep -il "IPADDR[[:space:]]\+${ip}" "${CONFIGS_DIR}"/*.peer 2>/dev/null && break
  done | head -n1
)

[ -n "$PEER_FILE" ] || { log "No peer file matching detected IPs"; exit 1; }


PEER_UUID=$(basename "$PEER_FILE" .peer)
log "Using PEER_UUID=${PEER_UUID}"

STORE=$(grep -E '^STORE[[:space:]]+' "$PEER_FILE" | awk '{print $2}')
[ -n "$STORE" ] || { log "STORE not found in $PEER_FILE"; exit 1; }

log "Using STORE=${STORE}"

# ---- Start pmdb server ----

log "Starting CTLPlane_pmdbServer"

"${LIBEXEC_DIR}/CTLPlane_pmdbServer" \
    -r "${RAFT_UUID}" \
    -u "${PEER_UUID}" \
    -g "${CONFIGS_DIR}/gossipNodes" \
    -l "${LOG_DIR}/pmdb_server_${PEER_UUID}.log" \
    -ll "Trace" \
    -p 1 \
    > "${LOG_DIR}/pmdb_server_${PEER_UUID}_stdouterr" 2>&1 &

log "pmdb server launched (PID $!)"

sleep 5

# ---- Start proxy client ----

CUUID=$(uuidgen)
log "Starting CTLPlane_proxy with client UUID=${CUUID}"

# Start CTLPlane_proxy in background
"${LIBEXEC_DIR}/CTLPlane_proxy" \
    -r "${RAFT_UUID}" \
    -u "${CUUID}" \
    -pa "${CONFIGS_DIR}/gossipNodes" \
    -n "Node_${CUUID}" \
    -mp 9701 \
    -l "${LOG_DIR}/pmdb_client_${CUUID}.log" \
    -ll "Trace" \
    > "${LOG_DIR}/pmdb_client_${CUUID}_stdouterr" 2>&1 &

log "CTLPlane_proxy started"

log "Starting cp-monitor tool"

# Start cp-monitor in background (parallel)
"${LIBEXEC_DIR}/cp-monitor" \
    -db "${STORE}/db" \
    > "${LOG_DIR}/cp_monitor_${CUUID}_stdouterr" 2>&1 &

log "cp-monitor started"