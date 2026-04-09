DIR ?= /tmp
CGO_LDFLAGS=-L$(DIR)/lib/ -L/usr/local/lib -llz4
CGO_CFLAGS=-I$(DIR)/include/ -I/usr/local/include/
LD_LIBRARY_PATH=$(DIR)/niova-core/lib/:$(DIR)/lib/

export CGO_LDFLAGS
export CGO_CFLAGS
export LD_LIBRARY_PATH

install_all: compile pmdbserver proxyserver ncpcclient configapp testapp niova-ctl monitor ccManager install

install_only: pmdbserver proxyserver ncpcclient configapp testapp niova-ctl monitor ccManager install

compile: vet
	echo "Compiling controlPlane"
	mkdir -p libexec
	go mod tidy

pmdbserver:
	go build -buildvcs=false -o libexec/CTLPlane_pmdbServer ./controlplane/pmdbServer/

proxyserver:
	go build -buildvcs=false -o libexec/CTLPlane_proxy ./controlplane/proxy/

ncpcclient:
	go build -buildvcs=false -o libexec/ncpc ./controlplane/ncpc/

configapp:
	go build -buildvcs=false -o libexec/cfgApp ./controlplane/configApplication/

testapp:
	go build -buildvcs=false -o libexec/testApp ./controlplane/testApplication/

niova-ctl:
	go build -buildvcs=false -o libexec/niova-ctl ./controlplane/niova-ctl/

monitor: 
	go build -buildvcs=false -o libexec/cp-monitor ./controlplane/monitor/

ccManager: 
	go build -buildvcs=false -o libexec/cc-manager ./controlplane/containerConfigManager/

install:
	cp libexec/CTLPlane_pmdbServer $(DIR)/libexec/niova/CTLPlane_pmdbServer
	cp libexec/CTLPlane_proxy $(DIR)/libexec/niova/CTLPlane_proxy
	cp libexec/ncpc $(DIR)/libexec/niova/ncpc
	cp libexec/cfgApp $(DIR)/libexec/niova/cfgApp
	cp libexec/testApp $(DIR)/libexec/niova/testApp
	cp libexec/cp-monitor $(DIR)/libexec/niova/cp-monitor
	cp libexec/cc-manager $(DIR)/libexec/niova/cc-manager
	cp libexec/niova-ctl $(DIR)/libexec/niova/niova-ctl
	cp scripts/docker/Dockerfile $(DIR)
	cp scripts/docker/controlplane.sh $(DIR)
	cp scripts/docker/raft-config.sh $(DIR)
	cp scripts/docker/run-cpcontainer.sh $(DIR)	
clean:
	rm -rf libexec

# Auto-format all Go source files under controlplane/
fmt:
	gofmt -w ./controlplane/
	@echo "gofmt done."

# Run go vet (uses the same CGO env set at the top of this file)
vet:
	go vet ./controlplane/...
	@echo "go vet done."

# golangci-lint must be installed: https://golangci-lint.run/docs/welcome/install/
lint: fmt vet
	GOTOOLCHAIN=local golangci-lint run --config .golangci.yml --timeout 10m ./controlplane/...
	@echo "All lint checks done."

help:
	@echo "Targets: install_all install_only compile fmt vet lint clean"

.PHONY: install_all install_only compile pmdbserver proxyserver ncpcclient \
        configapp testapp niova-ctl monitor ccManager install clean \
        fmt vet lint

# ---------------------------------------------------------------------------
# RPM packaging targets — build-in-place mode
#
# rpmbuild compiles directly inside this repo. No source tarball is needed.
# The build machine must have git and repo access (for submodule init).
#
# Usage:
#   make rpm-x86_64  VERSION=1.0.0
#   make rpm-aarch64 VERSION=1.0.0
#
# Output RPMs are written to rpmbuild/RPMS/<arch>/
# ---------------------------------------------------------------------------
VERSION ?= 1.0.0

# ---------------------------------------------------------------------------
# niova-core RPM — must be built and installed before niova-mdsvc RPM
# ---------------------------------------------------------------------------
rpm-core-x86_64:
	mkdir -p rpmbuild/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}
	rpmbuild -ba packaging/niova-core-x86_64.spec \
	    --build-in-place \
	    --define "_topdir $(CURDIR)/rpmbuild" \
	    --define "_builddir $(CURDIR)" \
	    --define "version $(VERSION)"
	@echo "RPM built: rpmbuild/RPMS/x86_64/ (niova-core)"

rpm-core-aarch64:
	mkdir -p rpmbuild/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}
	rpmbuild -ba packaging/niova-core-aarch64.spec \
	    --build-in-place \
	    --define "_topdir $(CURDIR)/rpmbuild" \
	    --define "_builddir $(CURDIR)" \
	    --define "version $(VERSION)"
	@echo "RPM built: rpmbuild/RPMS/aarch64/ (niova-core)"

# ---------------------------------------------------------------------------
# niova-mdsvc RPM — requires niova-core to be installed on the build host
# ---------------------------------------------------------------------------
rpm-x86_64:
	mkdir -p rpmbuild/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}
	rpmbuild -ba packaging/niova-mdsvc-x86_64.spec \
	    --build-in-place \
	    --define "_topdir $(CURDIR)/rpmbuild" \
	    --define "_builddir $(CURDIR)" \
	    --define "niova_core $(shell if [ -d $(CURDIR)/niova-core-build ]; then echo $(CURDIR)/niova-core-build; else echo /var/niova; fi)" \
	    --define "version $(VERSION)"
	@echo "RPM built: rpmbuild/RPMS/x86_64/ (niova-mdsvc)"

rpm-aarch64:
	mkdir -p rpmbuild/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}
	rpmbuild -ba packaging/niova-mdsvc-aarch64.spec \
	    --build-in-place \
	    --define "_topdir $(CURDIR)/rpmbuild" \
	    --define "_builddir $(CURDIR)" \
	    --define "niova_core $(shell if [ -d $(CURDIR)/niova-core-build ]; then echo $(CURDIR)/niova-core-build; else echo /var/niova; fi)" \
	    --define "version $(VERSION)"
	@echo "RPM built: rpmbuild/RPMS/aarch64/ (niova-mdsvc)"
