#!/usr/bin/env bash
# build-c-deps.sh
#
# Builds the full C library dependency chain:
#   libbacktrace -> niova-core -> niova-raft -> niova-pumicedb
#
# All libraries are installed into PREFIX so that the Go CGO build
# can find them via CGO_LDFLAGS and CGO_CFLAGS.
#
# Usage: bash packaging/build-c-deps.sh <PREFIX> [<JOBS>]
#   PREFIX  Install destination for all C libraries and headers.
#   JOBS    Parallel make jobs (default: nproc).

set -euo pipefail

PREFIX="${1:?Usage: build-c-deps.sh <PREFIX> [JOBS]}"
JOBS="${2:-$(nproc)}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PUMICEDB_DIR="${REPO_ROOT}/modules/niova-pumicedb"
RAFT_DIR="${PUMICEDB_DIR}/modules/niova-raft"
CORE_DIR="${RAFT_DIR}/modules/niova-core"
BACKTRACE_DIR="${PUMICEDB_DIR}/modules/libbacktrace"

log() { echo "[build-c-deps] $*"; }

# Abort early if submodules are not initialized
if [ ! -f "${CORE_DIR}/configure.ac" ] && [ ! -f "${CORE_DIR}/configure.in" ]; then
    echo "ERROR: niova-core submodule not initialized." >&2
    echo "Run: git submodule update --init --recursive" >&2
    exit 1
fi

mkdir -p "${PREFIX}"

# ── libbacktrace ─────────────────────────────────────────────────────────────
log "Building libbacktrace -> ${PREFIX}"
cd "${BACKTRACE_DIR}"
./configure --prefix="${PREFIX}"
make -j"${JOBS}"
make install
log "libbacktrace done"

# ── niova-core ───────────────────────────────────────────────────────────────
log "Building niova-core -> ${PREFIX}"
cd "${CORE_DIR}"
./prepare.sh
./configure \
    --enable-devel \
    --prefix="${PREFIX}"
make -j"${JOBS}"
make install
log "niova-core done"

# ── niova-raft ───────────────────────────────────────────────────────────────
log "Building niova-raft -> ${PREFIX}"
cd "${RAFT_DIR}"
./prepare.sh
./configure \
    --with-niova="${PREFIX}" \
    --prefix="${PREFIX}" \
    --enable-devel
make clean
make -j"${JOBS}"
make install
log "niova-raft done"

# ── niova-pumicedb ───────────────────────────────────────────────────────────
log "Building niova-pumicedb -> ${PREFIX}"
cd "${PUMICEDB_DIR}"
./prepare.sh
./configure \
    --enable-devel \
    --with-niova="${PREFIX}" \
    --prefix="${PREFIX}"
make clean
make -j"${JOBS}"
make install
log "niova-pumicedb done"

log "All C dependencies installed to: ${PREFIX}"
log "Libraries: $(ls ${PREFIX}/lib/*.so* 2>/dev/null | wc -l) .so files"
