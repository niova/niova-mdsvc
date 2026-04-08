ARG VERSION=1.1.0

# ── Stage 1: Base image with dependencies ───────────────────────────────────
# Use the pre-built dependency image. 
# Build this image once with: docker build -t niova-mdsvc-deps -f docker-build/Dockerfile.deps .
FROM niova-mdsvc-deps:latest as base
ARG VERSION
ENV VERSION=${VERSION}

# ── Stage 2: Create Build User and Directory ────────────────────────────────
RUN useradd -m builder
USER builder
WORKDIR /home/builder/niova-mdsvc

# ── Stage 3: Build niova-core ────────────────────────────────────────────────
FROM base as core-builder
ARG VERSION
COPY --chown=builder:builder . .

# Initialize submodules (required for builds)
RUN git config --global --add safe.directory /home/builder/niova-mdsvc && \
    git submodule update --init --recursive

# Build niova-core RPM
RUN make rpm-core-$(uname -m) VERSION=${VERSION}

# ── Stage 4: Build niova-mdsvc ───────────────────────────────────────────────
FROM core-builder as mdsvc-builder
ARG VERSION

# Install the niova-core RPM built in the previous stage
USER root
RUN ARCH=$(uname -m) && dnf install -y rpmbuild/RPMS/${ARCH}/niova-core-${VERSION}-1.el10.${ARCH}.rpm
USER builder

# Build niova-mdsvc RPM
RUN make rpm-$(uname -m) VERSION=${VERSION}

# ── Stage 5: Artifact Extraction ─────────────────────────────────────────────
FROM base as artifacts
WORKDIR /rpms
COPY --from=mdsvc-builder /home/builder/niova-mdsvc/rpmbuild/RPMS/*/*.rpm ./
