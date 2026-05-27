# Niova Docker RPM Build System

This directory contains a refactored Dockerized build system for creating Niova RPMs on Rocky Linux 10. The system uses a dedicated dependencies image to cache heavy builds (`curl`, `libbacktrace`, `go`, etc.) and speed up subsequent RPM builds.

## Prerequisites
- Docker installed and running.
- Access to the Niova source code.

## Build Process

The build is split into two parts:

1.  **Dependencies Image (`Dockerfile.deps`):**
    - Installs all system-level dependencies (DNF packages, Development Tools).
    - Builds and installs `curl 8.6.0` from source.
    - Builds and installs `libbacktrace` from source.
    - Installs `Go 1.24+`.
    - This image is built once and reused.

2.  **RPM Build Image (`Dockerfile`):**
    - Uses the dependencies image as a base.
    - Builds the `niova-core` RPM.
    - Installs `niova-core` and builds the `niova-mdsvc` RPM.
    - Extracts the final RPMs.

## Usage

### 1. Build the RPMs
Run the `build.sh` script. It will automatically build the dependencies image if it is missing.

```bash
chmod +x build.sh verify.sh
./build.sh
```

To force a rebuild of the dependencies image (e.g., after updating dependencies), use:
```bash
./build.sh --build-deps
```

### 2. Verify the Installation
Run the `verify.sh` script to test the installation of the generated RPMs in a clean container.

```bash
./verify.sh
```

## Produced Artifacts
The resulting RPMs will be placed in the `docker-build/rpms/` directory.
- `niova-core-<version>-1.el10.<arch>.rpm`
- `niova-mdsvc-<version>-1.el10.<arch>.rpm`
