#!/bin/bash
# deploy.sh - CTLPlane CP deployment script (post-RPM install)

set -e

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

# ---- Validations ----
[ "${#NODES[@]}" -gt 0 ] || { echo "No nodes found in config"; exit 1; }
[ -n "$PEER_PORT" ]         || { echo "peer port missing in config"; exit 1; }
[ -n "$CLIENT_PORT" ]       || { echo "client port missing in config"; exit 1; }
[ -n "$START_GOSSIP_PORT" ] || { echo "gossip start port missing in config"; exit 1; }
[ -n "$END_GOSSIP_PORT" ]   || { echo "gossip end port missing in config"; exit 1; }
[ -n "$OUTPUT_DIR" ]        || { echo "output_dir missing in config"; exit 1; }
[ -n "$LIB_DIR" ]           || { echo "lib_dir missing in config"; exit 1; }
[ -n "$BIN_DIR" ]           || { echo "bin_dir missing in config"; exit 1; }

# ---- Derived paths ----
RAFT_CONFIG_DIR="${OUTPUT_DIR}/config"
LIBEXEC_DIR="${BIN_DIR}"
LOG_DIR="${OUTPUT_DIR}/log"
DB_DIR="${OUTPUT_DIR}/db"

# ---- Env defaults ----
: "${LD_LIBRARY_PATH:=${LIB_DIR}}"
: "${NIOVA_INOTIFY_BASE_PATH:=${OUTPUT_DIR}/ctl-interface}"
NIOVA_APPLY_HANDLER_VERSION=0
USER_ENCRYPTION_KEY="81gavMyXh9dEMT7kM7gR+gS79ovzPwyjWmV1VA/TUII"

export LD_LIBRARY_PATH
export NIOVA_INOTIFY_BASE_PATH
export NIOVA_APPLY_HANDLER_VERSION
export USER_ENCRYPTION_KEY

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
# INIT MODE: cleanup + recreate directories
# ============================================================
if [ "$MODE" = "init" ]; then
    log "Init mode: cleaning old state under ${OUTPUT_DIR}"

    rm -rf "${RAFT_CONFIG_DIR}" "${LOG_DIR}" "${DB_DIR}" "${OUTPUT_DIR}/ctl-interface"

    mkdir -p "${RAFT_CONFIG_DIR}" "${LOG_DIR}" "${DB_DIR}" "${OUTPUT_DIR}/ctl-interface"
else
    mkdir -p "${RAFT_CONFIG_DIR}" "${LOG_DIR}" "${DB_DIR}" "${OUTPUT_DIR}/ctl-interface"
fi

# ============================================================
# RESTART MODE: fix STORE paths
# ============================================================
if [ "$MODE" = "restart" ]; then
    update_store_paths "${RAFT_CONFIG_DIR}" "${DB_DIR}"
fi

# ============================================================
# INIT MODE: generate Raft configs
# ============================================================
if [ "$MODE" = "init" ]; then
    log "Generating Raft configs in ${RAFT_CONFIG_DIR}"

    RAFT_UUID=$(uuidgen)
    RAFT_FILE="${RAFT_CONFIG_DIR}/${RAFT_UUID}.raft"
    echo "RAFT ${RAFT_UUID}" > "$RAFT_FILE"

    idx=0
    for node in "${NODES[@]}"; do
        PEER_UUID=$(uuidgen)
        echo "PEER ${PEER_UUID}" >> "$RAFT_FILE"

        if [ "$TYPE" = "localhost" ]; then
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

        log "peer ${PEER_UUID} ip=${PEER_IP} port=${NODE_PEER_PORT}"
        idx=$((idx + 1))
    done

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
fi

# ---- Locate RAFT UUID ----
RAFT_FILE=$(ls -1 "${RAFT_CONFIG_DIR}"/*.raft 2>/dev/null | head -n1)
[ -n "$RAFT_FILE" ] || { echo "No .raft file found. Run with -m init"; exit 1; }

RAFT_UUID=$(basename "$RAFT_FILE" .raft)
log "Using RAFT_UUID=${RAFT_UUID}"

: "${NIOVA_LOCAL_CTL_SVC_DIR:=${RAFT_CONFIG_DIR}}"
export NIOVA_LOCAL_CTL_SVC_DIR

# ============================================================
# LOCALHOST MODE
# ============================================================
if [ "$TYPE" = "localhost" ]; then
    for peer_file in "${RAFT_CONFIG_DIR}"/*.peer; do
        peer_uuid=$(basename "$peer_file" .peer)

        "${LIBEXEC_DIR}/CTLPlane_pmdbServer" \
            -r "${RAFT_UUID}" \
            -u "${peer_uuid}" \
            -g "${RAFT_CONFIG_DIR}/gossipNodes" \
            -l "${LOG_DIR}/pmdb_server_${peer_uuid}.log" \
            -ll "Trace" \
            -p 1 \
            > "${LOG_DIR}/pmdb_server_${peer_uuid}_stdouterr" 2>&1 &
    done

    sleep 5

    CUUID=$(uuidgen)
    "${LIBEXEC_DIR}/CTLPlane_proxy" \
        -r "${RAFT_UUID}" \
        -u "${CUUID}" \
        -pa "${RAFT_CONFIG_DIR}/gossipNodes" \
        -n "Node_${CUUID}" \
        -l "${LOG_DIR}/pmdb_client_${CUUID}.log" \
        -ll "Trace" \
        > "${LOG_DIR}/pmdb_client_${CUUID}_stdouterr" 2>&1 &

# ============================================================
# MULTINODE MODE
# ============================================================
else
    PDSH_HOSTS=$(IFS=','; echo "${NODES[*]}")

    for node in "${NODES[@]}"; do
        rsync -az "${RAFT_CONFIG_DIR}/" "${node}:${RAFT_CONFIG_DIR}/"
    done

    pdsh -w "${PDSH_HOSTS}" bash <<PDSH_CMD
export NIOVA_LOCAL_CTL_SVC_DIR='${NIOVA_LOCAL_CTL_SVC_DIR}'
export LD_LIBRARY_PATH='${LD_LIBRARY_PATH}'
export NIOVA_INOTIFY_BASE_PATH='${NIOVA_INOTIFY_BASE_PATH}'

RAFT_CONFIG_DIR='${RAFT_CONFIG_DIR}'
LIBEXEC_DIR='${LIBEXEC_DIR}'
LOG_DIR='${LOG_DIR}'
RAFT_UUID='${RAFT_UUID}'

IPS=\$(hostname -I | tr ' ' '\n')
PEER_FILE=\$(for ip in \$IPS; do grep -il "IPADDR[[:space:]]\+\$ip" "\$RAFT_CONFIG_DIR"/*.peer && break; done)

PEER_UUID=\$(basename "\$PEER_FILE" .peer)

"\$LIBEXEC_DIR/CTLPlane_pmdbServer" -r "\$RAFT_UUID" -u "\$PEER_UUID" \
-g "\$RAFT_CONFIG_DIR/gossipNodes" \
-l "\$LOG_DIR/pmdb_server_\$PEER_UUID.log" \
-ll Trace -p 1 > "\$LOG_DIR/out" 2>&1 &

sleep 5

CUUID=\$(uuidgen)
"\$LIBEXEC_DIR/CTLPlane_proxy" -r "\$RAFT_UUID" -u "\$CUUID" \
-pa "\$RAFT_CONFIG_DIR/gossipNodes" \
-n "Node_\$CUUID" \
-l "\$LOG_DIR/pmdb_client_\$CUUID.log" \
-ll Trace > "\$LOG_DIR/out2" 2>&1 &
PDSH_CMD

fi

log "Deployment complete"
