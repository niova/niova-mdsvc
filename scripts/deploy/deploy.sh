#!/bin/bash
# deploy.sh - CTLPlane CP deployment script (post-RPM install)
#
# Usage: sudo ./deploy.sh [-m init|restart] [-t localhost|multinode] <config.yaml>
#
# -m init     : generate Raft configs then start processes
# -m restart  : start processes using existing configs (default)
# -t localhost: start all pmdb servers + one proxy on this machine (default)
# -t multinode: start one pmdb + one proxy per node via pdsh

set -e

# ---- Fixed paths (post-RPM install) ----
RAFT_CONFIG_DIR="/var/niova/config"
LIBEXEC_DIR="/usr/local/libexec/niova"
LOG_DIR="/var/log/niova"
DB_DIR="/var/niova/db"

# ---- Env defaults (user-exported values take precedence) ----
# NIOVA_LOCAL_CTL_SVC_DIR is set later once RAFT_UUID is known
: "${LD_LIBRARY_PATH:=/usr/local/lib}"
: "${NIOVA_INOTIFY_BASE_PATH:=/var/niova/ctl-interface}"
NIOVA_APPLY_HANDLER_VERSION=0
USER_ENCRYPTION_KEY="81gavMyXh9dEMT7kM7gR+gS79ovzPwyjWmV1VA/TUII"

# ---- Defaults ----
MODE="restart"
TYPE="localhost"

log() { echo "[deploy][$(date '+%Y-%m-%d %H:%M:%S')] $*"; }

usage() {
    echo "Usage: $0 [-m init|restart] [-t localhost|multinode] <config.yaml>"
    exit 1
}

# ---- Parse args ----
while getopts ":m:t:" opt; do
    case $opt in
        m) MODE="$OPTARG" ;;
        t) TYPE="$OPTARG" ;;
        :) echo "Option -${OPTARG} requires an argument."; usage ;;
        *) usage ;;
    esac
done
shift $((OPTIND - 1))

CFG_FILE="${1:-}"
[ -n "$CFG_FILE" ] || { echo "Config file required."; usage; }
CFG_FILE="$(readlink -f "$CFG_FILE")"
[ -f "$CFG_FILE" ] || { echo "Config file not found: $1"; exit 1; }

case "$MODE" in init|restart) ;; *) echo "Invalid mode: $MODE"; usage ;; esac
case "$TYPE" in localhost|multinode) ;; *) echo "Invalid type: $TYPE"; usage ;; esac

log "mode=${MODE}  type=${TYPE}  config=${CFG_FILE}"

# ---- Export env (NIOVA_LOCAL_CTL_SVC_DIR exported after RAFT_UUID is known) ----
export LD_LIBRARY_PATH
export NIOVA_INOTIFY_BASE_PATH
export NIOVA_APPLY_HANDLER_VERSION
export USER_ENCRYPTION_KEY

# ---- Parse config ----
mapfile -t NODES < <(awk '
    $1=="nodes:" {flag=1; next}
    flag && $1=="-" {print $2}
    flag && $1!~"-" {exit}
' "$CFG_FILE")

START_GOSSIP_PORT=$(awk '/start:/ {print $2}' "$CFG_FILE")
END_GOSSIP_PORT=$(awk '/end:/ {print $2}' "$CFG_FILE")
PEER_PORT=$(awk '/peer:/ {print $2}' "$CFG_FILE")
CLIENT_PORT=$(awk '/client:/ {print $2}' "$CFG_FILE")

[ "${#NODES[@]}" -gt 0 ] || { echo "No nodes found in config"; exit 1; }
[ -n "$PEER_PORT" ]         || { echo "peer port missing in config"; exit 1; }
[ -n "$CLIENT_PORT" ]       || { echo "client port missing in config"; exit 1; }
[ -n "$START_GOSSIP_PORT" ] || { echo "gossip start port missing in config"; exit 1; }
[ -n "$END_GOSSIP_PORT" ]   || { echo "gossip end port missing in config"; exit 1; }

# ============================================================
# INIT MODE: generate Raft configs
# ============================================================
if [ "$MODE" = "init" ]; then
    log "Generating Raft configs in ${RAFT_CONFIG_DIR}"
    mkdir -p "${RAFT_CONFIG_DIR}"

    # Remove stale configs
    rm -f "${RAFT_CONFIG_DIR}"/*.raft "${RAFT_CONFIG_DIR}"/*.peer \
          "${RAFT_CONFIG_DIR}"/gossipNodes

    RAFT_UUID=$(uuidgen)
    RAFT_FILE="${RAFT_CONFIG_DIR}/${RAFT_UUID}.raft"
    echo "RAFT ${RAFT_UUID}" > "$RAFT_FILE"

    idx=0
    for node in "${NODES[@]}"; do
        PEER_UUID=$(uuidgen)
        echo "PEER ${PEER_UUID}" >> "$RAFT_FILE"

        if [ "$TYPE" = "localhost" ]; then
            # All peers on 127.0.0.1; offset ports to avoid collisions
            PEER_IP="127.0.0.1"
            NODE_PEER_PORT=$((PEER_PORT + idx))
            NODE_CLIENT_PORT=$((CLIENT_PORT + idx))
        else
            PEER_IP="$node"
            NODE_PEER_PORT="$PEER_PORT"
            NODE_CLIENT_PORT="$CLIENT_PORT"
        fi

        cat > "${RAFT_CONFIG_DIR}/${PEER_UUID}.peer" <<EOF
RAFT         ${RAFT_UUID}
IPADDR       ${PEER_IP}
PORT         ${NODE_PEER_PORT}
CLIENT_PORT  ${NODE_CLIENT_PORT}
STORE        ${DB_DIR}/${PEER_UUID}.raftdb
EOF
        log "  peer ${PEER_UUID}  ip=${PEER_IP}  port=${NODE_PEER_PORT}  client=${NODE_CLIENT_PORT}"
        idx=$((idx + 1))
    done

    # gossipNodes: space-separated IPs on line 1, port range on line 2
    if [ "$TYPE" = "localhost" ]; then
        GOSSIP_IPS=""
        for (( i=0; i<${#NODES[@]}; i++ )); do
            [ -z "${GOSSIP_IPS}" ] && GOSSIP_IPS="127.0.0.1" || GOSSIP_IPS="${GOSSIP_IPS} 127.0.0.1"
        done
        echo "${GOSSIP_IPS}" > "${RAFT_CONFIG_DIR}/gossipNodes"
    else
        echo "${NODES[*]}" > "${RAFT_CONFIG_DIR}/gossipNodes"
    fi
    echo "${START_GOSSIP_PORT} ${END_GOSSIP_PORT}" >> "${RAFT_CONFIG_DIR}/gossipNodes"

    log "Raft configs written to ${RAFT_CONFIG_DIR}"
fi

# ============================================================
# Locate Raft UUID (required for both modes)
# ============================================================
RAFT_FILE=$(ls -1 "${RAFT_CONFIG_DIR}"/*.raft 2>/dev/null | head -n1)
[ -n "$RAFT_FILE" ] || {
    log "No .raft file found in ${RAFT_CONFIG_DIR}. Run with -m init first."
    exit 1
}
RAFT_UUID=$(basename "$RAFT_FILE" .raft)
log "Using RAFT_UUID=${RAFT_UUID}"

# Set NIOVA_LOCAL_CTL_SVC_DIR to raft config dir if not user-exported
: "${NIOVA_LOCAL_CTL_SVC_DIR:=${RAFT_CONFIG_DIR}}"
export NIOVA_LOCAL_CTL_SVC_DIR
log "NIOVA_LOCAL_CTL_SVC_DIR=${NIOVA_LOCAL_CTL_SVC_DIR}"

mkdir -p "$LOG_DIR"

# ============================================================
# LOCALHOST: start all pmdb servers + one proxy locally
# ============================================================
if [ "$TYPE" = "localhost" ]; then
    log "Starting pmdb servers on localhost"

    for peer_file in "${RAFT_CONFIG_DIR}"/*.peer; do
        peer_uuid=$(basename "$peer_file" .peer)
        store=$(awk '/^STORE/ {print $2}' "$peer_file")
        mkdir -p "$(dirname "$store")"

        log "  Starting CTLPlane_pmdbServer peer=${peer_uuid}"
        "${LIBEXEC_DIR}/CTLPlane_pmdbServer" \
            -r "${RAFT_UUID}" \
            -u "${peer_uuid}" \
            -g "${RAFT_CONFIG_DIR}/gossipNodes" \
            -l "${LOG_DIR}/pmdb_server_${peer_uuid}.log" \
            -ll "Trace" \
            -p 1 \
            > "${LOG_DIR}/pmdb_server_${peer_uuid}_stdouterr" 2>&1 &
        log "  pmdb server ${peer_uuid} started (PID $!)"
    done

    sleep 5

    CUUID=$(uuidgen)
    log "Starting CTLPlane_proxy (UUID=${CUUID})"
    "${LIBEXEC_DIR}/CTLPlane_proxy" \
        -r "${RAFT_UUID}" \
        -u "${CUUID}" \
        -pa "${RAFT_CONFIG_DIR}/gossipNodes" \
        -n "Node_${CUUID}" \
        -l "${LOG_DIR}/pmdb_client_${CUUID}.log" \
        -ll "Trace" \
        > "${LOG_DIR}/pmdb_client_${CUUID}_stdouterr" 2>&1 &
    log "CTLPlane_proxy started (PID $!)"

# ============================================================
# MULTINODE: use pdsh to start one pmdb + one proxy per node
# ============================================================
else
    PDSH_HOSTS=$(IFS=','; echo "${NODES[*]}")

    log "Syncing Raft configs to all nodes"
    for node in "${NODES[@]}"; do
        rsync -az "${RAFT_CONFIG_DIR}/" "${node}:${RAFT_CONFIG_DIR}/"
    done

    log "Starting pmdb + proxy on nodes: ${PDSH_HOSTS}"

    # Each remote node detects its own IP to find its peer file, then starts
    # one CTLPlane_pmdbServer and one CTLPlane_proxy.
    pdsh -w "${PDSH_HOSTS}" bash <<PDSH_CMD
set -e
export NIOVA_LOCAL_CTL_SVC_DIR='${NIOVA_LOCAL_CTL_SVC_DIR}'
export LD_LIBRARY_PATH='${LD_LIBRARY_PATH}'
export NIOVA_INOTIFY_BASE_PATH='${NIOVA_INOTIFY_BASE_PATH}'
export NIOVA_APPLY_HANDLER_VERSION=${NIOVA_APPLY_HANDLER_VERSION}
export USER_ENCRYPTION_KEY='${USER_ENCRYPTION_KEY}'

RAFT_CONFIG_DIR='${RAFT_CONFIG_DIR}'
LIBEXEC_DIR='${LIBEXEC_DIR}'
LOG_DIR='${LOG_DIR}'
RAFT_UUID='${RAFT_UUID}'

mkdir -p "\${LOG_DIR}"

# Match this node's IP to a peer file
IPS=\$(hostname -I | tr ' ' '\n')
PEER_FILE=\$(
    for ip in \$IPS; do
        grep -il "IPADDR[[:space:]]\+\${ip}" "\${RAFT_CONFIG_DIR}"/*.peer 2>/dev/null && break
    done | head -n1
)
[ -n "\${PEER_FILE}" ] || { echo "No peer file matching local IPs"; exit 1; }

PEER_UUID=\$(basename "\${PEER_FILE}" .peer)
STORE=\$(awk '/^STORE/ {print \$2}' "\${PEER_FILE}")
mkdir -p "\$(dirname "\${STORE}")"

echo "Starting CTLPlane_pmdbServer peer=\${PEER_UUID}"
"\${LIBEXEC_DIR}/CTLPlane_pmdbServer" \
    -r "\${RAFT_UUID}" \
    -u "\${PEER_UUID}" \
    -g "\${RAFT_CONFIG_DIR}/gossipNodes" \
    -l "\${LOG_DIR}/pmdb_server_\${PEER_UUID}.log" \
    -ll "Trace" \
    -p 1 \
    > "\${LOG_DIR}/pmdb_server_\${PEER_UUID}_stdouterr" 2>&1 &

sleep 5

CUUID=\$(uuidgen)
echo "Starting CTLPlane_proxy UUID=\${CUUID}"
"\${LIBEXEC_DIR}/CTLPlane_proxy" \
    -r "\${RAFT_UUID}" \
    -u "\${CUUID}" \
    -pa "\${RAFT_CONFIG_DIR}/gossipNodes" \
    -n "Node_\${CUUID}" \
    -l "\${LOG_DIR}/pmdb_client_\${CUUID}.log" \
    -ll "Trace" \
    > "\${LOG_DIR}/pmdb_client_\${CUUID}_stdouterr" 2>&1 &
PDSH_CMD

fi

log "Deployment complete (mode=${MODE}, type=${TYPE})"
