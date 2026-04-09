#!/bin/bash
# deploy.sh - CTLPlane CP deployment script (FINAL FIXED)

set -euo pipefail

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
[ -n "$CFG_FILE" ] || usage
CFG_FILE="$(readlink -f "$CFG_FILE")"
[ -f "$CFG_FILE" ] || { echo "Config not found: $CFG_FILE"; exit 1; }

case "$MODE" in init|restart) ;; *) usage ;; esac
case "$TYPE" in localhost|multinode) ;; *) usage ;; esac

log "mode=$MODE type=$TYPE config=$CFG_FILE"

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

OUTPUT_DIR=$(awk '/output_dir:/ {print $2}' "$CFG_FILE")
LIB_DIR=$(awk '/lib_dir:/ {print $2}' "$CFG_FILE")
BIN_DIR=$(awk '/bin_dir:/ {print $2}' "$CFG_FILE")

# ---- Validate ----
[ "${#NODES[@]}" -gt 0 ] || exit 1
[ -n "$PEER_PORT" ] || exit 1
[ -n "$CLIENT_PORT" ] || exit 1
[ -n "$OUTPUT_DIR" ] || exit 1
[ -n "$LIB_DIR" ] || exit 1
[ -n "$BIN_DIR" ] || exit 1

# ---- Paths ----
RAFT_CONFIG_DIR="${OUTPUT_DIR}/config"
LOG_DIR="${OUTPUT_DIR}/log"
DB_DIR="${OUTPUT_DIR}/db"

mkdir -p "$RAFT_CONFIG_DIR" "$LOG_DIR" "$DB_DIR" "${OUTPUT_DIR}/ctl-interface"

# ---- Env ----
export LD_LIBRARY_PATH="${LIB_DIR}:${LD_LIBRARY_PATH:-}"
export NIOVA_INOTIFY_BASE_PATH="${OUTPUT_DIR}/ctl-interface"
export NIOVA_APPLY_HANDLER_VERSION=0
export USER_ENCRYPTION_KEY="81gavMyXh9dEMT7kM7gR+gS79ovzPwyjWmV1VA/TUII"




# ============================================================
# FUNCTION: update STORE paths (restart mode)
# ============================================================
update_store_paths() {
    local dir="$1"
    local newpath="$2"

    # remove trailing slash to avoid //
    newpath="${newpath%/}"

    log "Updating STORE paths in ${dir} to ${newpath}"

    for file in "${dir}"/*.peer; do
        [ -e "$file" ] || continue

        uuid="$(basename "$file" .peer)"
        tmp="$(mktemp)"

        awk -v p="$newpath" -v u="$uuid" '
        /^STORE/ {
            print "STORE        " p "/" u ".raftdb"
            next
        }
        { print }
        ' "$file" > "$tmp"

        mv "$tmp" "$file"
        chmod 0666 "$file"

        log "updated $file"
    done
}

# ============================================================
# INIT
# ============================================================
if [ "$MODE" = "init" ]; then
    log "Initializing..."

    rm -rf "$RAFT_CONFIG_DIR" "$LOG_DIR" "$DB_DIR" "${OUTPUT_DIR}/ctl-interface"
    mkdir -p "$RAFT_CONFIG_DIR" "$LOG_DIR" "$DB_DIR" "${OUTPUT_DIR}/ctl-interface"

    RAFT_UUID=$(uuidgen)
    echo "RAFT $RAFT_UUID" > "${RAFT_CONFIG_DIR}/${RAFT_UUID}.raft"

    idx=0
    for node in "${NODES[@]}"; do
        PEER_UUID=$(uuidgen)

        if [ "$TYPE" = "localhost" ]; then
            IP="127.0.0.1"
            PPORT=$((PEER_PORT + idx))
            CPORT=$((CLIENT_PORT + idx))
        else
            IP="$node"
            PPORT="$PEER_PORT"
            CPORT="$CLIENT_PORT"
        fi

        cat > "${RAFT_CONFIG_DIR}/${PEER_UUID}.peer" <<EOF
RAFT         ${RAFT_UUID}
IPADDR       ${IP}
PORT         ${PPORT}
CLIENT_PORT  ${CPORT}
STORE        ${DB_DIR}/${PEER_UUID}.raftdb
EOF

        idx=$((idx + 1))
    done

    echo "${NODES[*]}" > "${RAFT_CONFIG_DIR}/gossipNodes"
    echo "${START_GOSSIP_PORT} ${END_GOSSIP_PORT}" >> "${RAFT_CONFIG_DIR}/gossipNodes"
fi

RAFT_FILE=$(ls "${RAFT_CONFIG_DIR}"/*.raft | head -n1)
RAFT_UUID=$(basename "$RAFT_FILE" .raft)

log "RAFT_UUID=$RAFT_UUID"

# ============================================================
# RESTART MODE: fix STORE paths
# ============================================================
if [ "$MODE" = "restart" ]; then
    update_store_paths "${RAFT_CONFIG_DIR}" "${DB_DIR}"
fi


# ============================================================
# LOCAL MODE
# ============================================================
if [ "$TYPE" = "localhost" ]; then
    for peer in "${RAFT_CONFIG_DIR}"/*.peer; do
        uuid=$(basename "$peer" .peer)

        "${BIN_DIR}/CTLPlane_pmdbServer" -r "$RAFT_UUID" -u "$uuid" \
            -g "${RAFT_CONFIG_DIR}/gossipNodes" \
            -l "${LOG_DIR}/pmdb_server_${uuid}.log" -ll Trace > "${LOG_DIR}/pmdb_server_${uuid}_stdout.log" 2>&1 &

    done

    sleep 5

    PROXY_UUID=$(uuidgen)
    "${BIN_DIR}/CTLPlane_proxy" -r "$RAFT_UUID" \
        -u "$PROXY_UUID" \
        -pa "${RAFT_CONFIG_DIR}/gossipNodes" \
        -n "Node" \
        -l "${LOG_DIR}/proxy_${PROXY_UUID}.log" -ll Trace > "${LOG_DIR}/proxy_${PROXY_UUID}_stdout.log" 2>&1 &
fi

# ============================================================
# MULTINODE MODE (NFS-BASED)
# ============================================================
if [ "$TYPE" = "multinode" ]; then

    export PDSH_SSH_ARGS="-o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=no"
    PDSH_HOST_LIST=$(IFS=','; echo "${NODES[*]}")

    log "Preparing shared deployment script..."

    # Generate the shared remote start script on NFS
    REMOTE_SCRIPT="${OUTPUT_DIR}/remote_start.sh"
    cat > "$REMOTE_SCRIPT" <<EOF
#!/usr/bin/bash
set -euo pipefail

# Environment - using shared paths on NFS
export LD_LIBRARY_PATH="${LIB_DIR}:/usr/lib64:/usr/lib:\${LD_LIBRARY_PATH:-}"
export NIOVA_INOTIFY_BASE_PATH="${OUTPUT_DIR}/ctl-interface"
export NIOVA_APPLY_HANDLER_VERSION=0
export USER_ENCRYPTION_KEY="81gavMyXh9dEMT7kM7gR+gS79ovzPwyjWmV1VA/TUII"

log_remote() { echo "[remote][\$(hostname)][\$(date '+%Y-%m-%d %H:%M:%S')] \$*"; }

log_remote "Detecting identity..."

PEER_FILE=""
for IP in \$(hostname -I); do
    MATCH=\$(grep -il "IPADDR[[:space:]][[:space:]]*\$IP" "${RAFT_CONFIG_DIR}"/*.peer | head -n1 || true)
    if [ -n "\$MATCH" ]; then
        PEER_FILE="\$MATCH"
        break
    fi
done

if [ -z "\$PEER_FILE" ]; then
    log_remote "ERROR: No matching peer found for IPs: \$(hostname -I)"
    exit 1
fi

PEER_UUID=\$(basename "\$PEER_FILE" .peer)
log_remote "Found identity: \$PEER_UUID"

log_remote "Starting pmdbServer..."
nohup "${BIN_DIR}/CTLPlane_pmdbServer" -r "${RAFT_UUID}" -u "\$PEER_UUID" \
    -g "${RAFT_CONFIG_DIR}/gossipNodes" \
    -l "${LOG_DIR}/pmdb_server_\${PEER_UUID}.log" -ll Trace > "${LOG_DIR}/pmdb_server_\${PEER_UUID}_stdout.log" 2>&1 &

sleep 2

log_remote "Starting proxy..."
PROXY_UUID=\$(uuidgen)
nohup "${BIN_DIR}/CTLPlane_proxy" -r "${RAFT_UUID}" -u "\$PROXY_UUID" \
    -pa "${RAFT_CONFIG_DIR}/gossipNodes" \
    -n "Node" \
    -l "${LOG_DIR}/proxy_\${PROXY_UUID}.log" -ll Trace > "${LOG_DIR}/proxy_\${PROXY_UUID}_stdout.log" 2>&1 &

log_remote "Startup sequence complete."
EOF
    chmod +x "$REMOTE_SCRIPT"

    # In NFS mode, we assume directories and files are already visible to all nodes.
    # We just need to trigger the execution via pdsh.

    log "Starting cluster via pdsh (shared NFS paths)..."
    pdsh -t 5 -R ssh -w "$PDSH_HOST_LIST" "$REMOTE_SCRIPT"

fi

log "Deployment complete"