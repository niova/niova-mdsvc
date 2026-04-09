#!/bin/bash
# verify.sh
# Verified that the built RPMs can be installed in a clean Rocky Linux 10 container.

set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ ! -d "${DIR}/rpms" ]; then
    echo "ERROR: ${DIR}/rpms directory not found. Run build.sh first."
    exit 1
fi

echo "==> Starting verification container..."
# Use the same base image
CONTAINER_NAME="niova-verify-$(date +%s)"
docker run -d --name "${CONTAINER_NAME}" rockylinux/rockylinux:10-ubi-init

cleanup() {
    echo "==> Cleaning up container..."
    docker rm -f "${CONTAINER_NAME}"
}
trap cleanup EXIT

echo "==> Copying RPMs to container..."
docker cp "${DIR}/rpms" "${CONTAINER_NAME}:/tmp/"

echo "==> Enabling EPEL (for rocksdb-libs)..."
docker exec "${CONTAINER_NAME}" dnf install -y epel-release epel-next-release

echo "==> Installing niova-core RPM..."
docker exec "${CONTAINER_NAME}" dnf install -y /tmp/rpms/niova-core-*.rpm

echo "==> Installing niova-mdsvc RPM..."
docker exec "${CONTAINER_NAME}" dnf install -y /tmp/rpms/niova-mdsvc-*.rpm

echo "==> Verifying installation paths..."
docker exec "${CONTAINER_NAME}" ls -l /usr/local/lib/libniova.so
docker exec "${CONTAINER_NAME}" ls -l /usr/local/bin/niova/CTLPlane_pmdbServer

echo "==> Verifying systemd units..."
docker exec "${CONTAINER_NAME}" systemctl list-unit-files | grep niova

echo "==> SUCCESS: RPMs installed correctly."
