#!/bin/bash
# build.sh
# Drives the Docker build and extracts the resulting RPMs to the host.

set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${DIR}/.." && pwd)"

# Build the dependencies image if it doesn't exist
if [[ "$(docker images -q niova-mdsvc-deps:latest 2> /dev/null)" == "" ]] || [[ "$1" == "--build-deps" ]]; then
    echo "==> Building Niova Dependencies Image..."
    docker build -t niova-mdsvc-deps -f "${DIR}/Dockerfile.deps" "${REPO_ROOT}"
fi

# Build the docker image
echo "==> Building Niova RPMs in Docker..."
docker build -t niova-rpm-builder -f "${DIR}/Dockerfile" "${REPO_ROOT}"

# Extract the RPMs
echo "==> Extracting RPMs to host..."
mkdir -p "${DIR}/rpms"
docker create --name niova-extract niova-rpm-builder /bin/true
docker cp niova-extract:/rpms/. "${DIR}/rpms/"
docker rm niova-extract

echo "==> Done! RPMs are in ${DIR}/rpms"
ls -l "${DIR}/rpms"
