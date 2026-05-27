#!/bin/bash

# serf_gossip_interface.sh
# Mimics the behavior of serf_gossip_interface.c

# Log function for stderr
log() {
    echo "$@" >&2
}

# Configuration of cmd line args
if [ $# -ne 2 ]; then
    log "Error: Invalid number of arguments"
    log "Usage: $0 <gossip_path> <gossip_key>"
    exit 1
fi

GOSSIP_PATH=$1
GOSSIP_KEY=$2

# Check for required binaries
check_dependencies() {
    local missing=()
    local deps=("serf" "jq" "sed" "head")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            missing+=("$dep")
        fi
    done

    if [ ${#missing[@]} -gt 0 ]; then
        log "Error: The following required binaries are missing: ${missing[*]}"
        log ""
        log "Installation hints:"
        for dep in "${missing[@]}"; do
            case "$dep" in
                "serf")
                    log "  - serf: Install via your package manager or download from https://www.serf.io/downloads.html"
                    ;;
                "jq")
                    log "  - jq: Run 'sudo apt-get install jq' (Debian/Ubuntu) or 'sudo dnf install jq' (Fedora/RHEL)"
                    ;;
                "sed"|"head")
                    log "  - $dep: This is a standard Unix utility. Please install 'coreutils' or equivalent."
                    ;;
                *)
                    log "  - $dep: Consult your system package manager."
                    ;;
            esac
        done
        exit 1
    fi
}

check_dependencies

# Validation
if [ -z "$GOSSIP_PATH" ]; then
    log "Error: NIOVA_GOSSIP_PATH is not set."
    exit 1
fi

if [ -z "$GOSSIP_KEY" ]; then
    log "Error: NIOVA_GOSSIP_KEY is not set."
    exit 1
fi

if [ ! -f "$GOSSIP_PATH" ]; then
    log "Error: Gossip file not found at $GOSSIP_PATH"
    exit 1
fi

# 1. Parse the gossipNodes file
# Expected format:
# Line 1: space separated IPs
# Line 2: start_port end_port
IPS=$(head -n 1 "$GOSSIP_PATH")
read -r START_PORT END_PORT <<< "$(sed -n '2p' "$GOSSIP_PATH")"

if [ -z "$IPS" ] || [ -z "$START_PORT" ] || [ -z "$END_PORT" ]; then
    log "Error: Invalid gossipNodes file format in $GOSSIP_PATH"
    log "Line 1 should contain IPs, Line 2 should contain START_PORT and END_PORT."
    exit 1
fi

log "Gossip IPs: $IPS"
log "Port Range: $START_PORT - $END_PORT"

# 2. Iterate over ip addr and port ranges to connect with a serf agent
for IP in $IPS; do
    for (( PORT=START_PORT; PORT<=END_PORT; PORT++ )); do
        AGENT_ADDR="$IP:$PORT"
        
        # 3. Get the gossip data from the agent
        # We query the agent for members with tag Type=PROXY and status=alive
        RESULT=$(serf members -rpc-addr="$AGENT_ADDR" -rpc-auth="$GOSSIP_KEY" -tag Type=PROXY -format json -status alive 2>/dev/null)
        
        if [ $? -eq 0 ] && [ -n "$RESULT" ]; then
            # identify HTTP port of the proxy!
            # Using jq to extract the Proxy IP and its Hport tag
            PROXY_INFO=$(echo "$RESULT" | jq -r '.members[] | select(.status == "alive" and .tags.Type == "PROXY") | "\(.addr) \(.tags.Hport)"' | head -n 1)
            
            if [ -n "$PROXY_INFO" ]; then
                read -r ADDR HPORT <<< "$PROXY_INFO"
                
                # Strip port from addr if it exists (addr is usually IP:Port)
                PROXY_IP=${ADDR%:*}
                
                log "Connected with a serf agent $AGENT_ADDR"
                echo "PROXY_IP=$PROXY_IP"
                echo "PROXY_HTTP_PORT=$HPORT"
                exit 0
            fi
        fi
    done
done

log "Error: No alive serf agent found in the specified range."
exit 1
