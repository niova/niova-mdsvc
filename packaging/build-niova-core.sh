#!/usr/bin/env bash
# build-niova-core.sh
#
# Builds the niova-core C library dependency chain:
#   libbacktrace -> niova-core
#
# All libraries and headers are installed into PREFIX so that downstream
# packages (niova-raft, niova-pumicedb, niova-mdsvc) can locate them.
#
# Usage: bash packaging/build-niova-core.sh <PREFIX> [<JOBS>]
#   PREFIX  Install destination (default: /opt/niova-core)
#   JOBS    Parallel make jobs (default: nproc).

set -euo pipefail

PREFIX="${1:?Usage: build-niova-core.sh <PREFIX> [JOBS]}"
JOBS="${2:-$(nproc)}"
JOBS="${JOBS#-j}" # Strip leading -j if present

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PUMICEDB_DIR="${REPO_ROOT}/modules/niova-pumicedb"
RAFT_DIR="${PUMICEDB_DIR}/modules/niova-raft"
CORE_DIR="${RAFT_DIR}/modules/niova-core"
BACKTRACE_DIR="${PUMICEDB_DIR}/modules/libbacktrace"

log() { echo "[build-niova-core] $*"; }

# Abort early if submodules are not initialized
if [ ! -f "${CORE_DIR}/configure.ac" ] && [ ! -f "${CORE_DIR}/configure.in" ]; then
    echo "ERROR: niova-core submodule not initialized." >&2
    echo "Run: git submodule update --init --recursive" >&2
    exit 1
fi

mkdir -p "${PREFIX}"

# ── libbacktrace (Skipped: using system libbacktrace from Docker image) ───────
# log "Building libbacktrace -> ${PREFIX}"
# cd "${BACKTRACE_DIR}"
# CFLAGS="-fPIC" ./configure --prefix="${PREFIX}" --enable-shared
# make -j"${JOBS}"
# make install
# log "libbacktrace done"

# ── niova-core ────────────────────────────────────────────────────────────────
log "Building niova-core -> ${PREFIX}"
cd "${CORE_DIR}"
./prepare.sh
./configure \
    --enable-devel \
    --prefix="${PREFIX}"
make -j"${JOBS}"
make install
log "niova-core done"

log "niova-core installed to: ${PREFIX}"
log "Libraries: $(ls ${PREFIX}/lib/*.so* 2>/dev/null | wc -l) .so files"
