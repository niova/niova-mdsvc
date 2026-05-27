ARG VERSION=1.1.0

# ── Stage 1: Base image with dependencies ───────────────────────────────────
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

# Copy entire repo (INCLUDING submodules from host)
COPY --chown=builder:builder . .

# Build niova-core RPM
RUN make rpm-core-$(uname -m) VERSION=${VERSION}

# ── Stage 4: Build niova-mdsvc ───────────────────────────────────────────────
FROM core-builder as mdsvc-builder
ARG VERSION

USER root

# Install the niova-core RPM built in previous stage
RUN ARCH=$(uname -m) && \
    dnf install -y rpmbuild/RPMS/${ARCH}/niova-core-${VERSION}-1.el10.${ARCH}.rpm

USER builder

# Build niova-mdsvc RPM
RUN make rpm-$(uname -m) VERSION=${VERSION}

# ── Stage 5: Artifact Extraction ─────────────────────────────────────────────
FROM base as artifacts
WORKDIR /rpms

# Copy all built RPMs
COPY --from=mdsvc-builder /home/builder/niova-mdsvc/rpmbuild/RPMS/*/*.rpm ./