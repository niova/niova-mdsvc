# niova-mdsvc
Niova Metadata Service

## CI Pipeline

This repository includes a GitHub Actions pipeline that automatically builds and verifies RPMs for `x86_64` and `aarch64` architectures.

### Artifacts and Releases

- **Pull Requests**: RPMs are built and uploaded as workflow artifacts for testing.
- **Main Branch**: RPMs are built on every push to `main`.
- **Releases**: Pushing a version tag (e.g., `v1.1.0`) automatically creates a GitHub Release and attaches the built RPMs.

### Manual Build

You can also build the RPMs locally using Docker:

```bash
bash docker-build/build.sh --build-deps
```

The resulting RPMs will be in `docker-build/rpms/`.
