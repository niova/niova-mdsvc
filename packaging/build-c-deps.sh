#!/usr/bin/env bash
# build-c-deps.sh
#
# Builds the niova-mdsvc C library dependencies:
#   niova-raft -> niova-pumicedb
#
# Requires niova-core to be pre-installed (via the niova-core RPM or
# build-niova-core.sh). Pass its install prefix as NIOVA_CORE_PREFIX.
#
# All libraries are installed into PREFIX so that the Go CGO build
# can find them via CGO_LDFLAGS and CGO_CFLAGS.
#
# Usage: bash packaging/build-c-deps.sh <PREFIX> <NIOVA_CORE_PREFIX> [<JOBS>]
#   PREFIX            Install destination for niova-raft and niova-pumicedb.
#   NIOVA_CORE_PREFIX Where niova-core is installed (default: /opt/niova-core).
#   JOBS              Parallel make jobs (default: nproc).

set -euo pipefail

PREFIX="${1:?Usage: build-c-deps.sh <PREFIX> <NIOVA_CORE_PREFIX> [JOBS]}"
NIOVA_CORE_PREFIX="${2:-/opt/niova-core}"
JOBS="${3:-$(nproc)}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PUMICEDB_DIR="${REPO_ROOT}/modules/niova-pumicedb"
RAFT_DIR="${PUMICEDB_DIR}/modules/niova-raft"

log() { echo "[build-c-deps] $*"; }

# Verify niova-core is available
if [ ! -d "${NIOVA_CORE_PREFIX}/lib" ]; then
    echo "ERROR: niova-core not found at ${NIOVA_CORE_PREFIX}." >&2
    echo "Install the niova-core RPM or run build-niova-core.sh first." >&2
    exit 1
fi

mkdir -p "${PREFIX}"

# ── niova-raft ────────────────────────────────────────────────────────────────
log "Building niova-raft -> ${PREFIX} (using niova-core from ${NIOVA_CORE_PREFIX})"
cd "${RAFT_DIR}"
./prepare.sh
./configure \
    --with-niova="${NIOVA_CORE_PREFIX}" \
    --prefix="${PREFIX}" \
    --enable-devel
make clean
make -j"${JOBS}"
make install
log "niova-raft done"

# ── niova-pumicedb ────────────────────────────────────────────────────────────
log "Building niova-pumicedb -> ${PREFIX} (using niova-core from ${NIOVA_CORE_PREFIX})"
cd "${PUMICEDB_DIR}"
./prepare.sh
./configure \
    --enable-devel \
    --with-niova="${NIOVA_CORE_PREFIX}" \
    --prefix="${PREFIX}"
make clean
make -j"${JOBS}"
make install
log "niova-pumicedb done"

log "All mdsvc C dependencies installed to: ${PREFIX}"
log "Libraries: $(ls ${PREFIX}/lib/*.so* 2>/dev/null | wc -l) .so files"
