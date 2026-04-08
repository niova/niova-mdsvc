%global pkg_name     niova-core
%global niova_prefix /opt/niova-core
%global niova_build  %{_builddir}/niova-core-build

Name:           %{pkg_name}
Version:        %{version}
Release:        1%{?dist}
Summary:        Niova core C libraries (libbacktrace + niova-core)
License:        Apache-2.0
URL:            https://github.com/00pauln00/niova-mdsvc
ExclusiveArch:  aarch64

# ---------------------------------------------------------------------------
# Build-time requirements
# ---------------------------------------------------------------------------
BuildRequires:  gcc
BuildRequires:  gcc-c++
BuildRequires:  make
BuildRequires:  autoconf
BuildRequires:  automake
BuildRequires:  libtool
BuildRequires:  git
BuildRequires:  openssl-devel
BuildRequires:  libuuid-devel
BuildRequires:  libgcrypt-devel

# ---------------------------------------------------------------------------
# Runtime requirements
# ---------------------------------------------------------------------------
Requires:       openssl-libs
Requires:       libuuid

%description
Shared C libraries for the Niova distributed storage system (aarch64).

Provides:
  libniova*.so   - Niova core runtime library
  libbacktrace.so - Stack-trace helper library

Installed to %{niova_prefix} and registered with ldconfig via
/etc/ld.so.conf.d/niova-core.conf.

This package is a shared dependency for niova-mdsvc and niova-block.
Headers are included so downstream packages can build against this library.

# ---------------------------------------------------------------------------
# %prep
# Build-in-place mode: no tarball.
# ---------------------------------------------------------------------------
%prep
CORE_DIR="modules/niova-pumicedb/modules/niova-raft/modules/niova-core"
if [ ! -f "${CORE_DIR}/configure.ac" ] && [ ! -f "${CORE_DIR}/configure.in" ]; then
    echo "ERROR: Git submodules are not initialized." >&2
    echo "Run: git submodule update --init --recursive" >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# %build
# ---------------------------------------------------------------------------
%build

export GOARCH=arm64
export GOOS=linux

mkdir -p %{niova_build}
bash packaging/build-niova-core.sh %{niova_build} %{?_smp_mflags:-j}$(nproc)

# ---------------------------------------------------------------------------
# %install
# ---------------------------------------------------------------------------
%install

# ── C shared libraries (runtime) ─────────────────────────────────────────────
install -d %{buildroot}%{niova_prefix}/lib

find %{niova_build}/lib -maxdepth 1 -name '*.so*' -type f \
    -exec install -m 0755 {} %{buildroot}%{niova_prefix}/lib/ \;

find %{niova_build}/lib -maxdepth 1 -name '*.so*' -type l \
    -exec cp -a {} %{buildroot}%{niova_prefix}/lib/ \;

# ── Headers (needed for building downstream packages) ────────────────────────
install -d %{buildroot}%{niova_prefix}/include

if [ -d %{niova_build}/include ]; then
    cp -a %{niova_build}/include/. %{buildroot}%{niova_prefix}/include/
fi

# ── ldconfig drop-in ─────────────────────────────────────────────────────────
install -d %{buildroot}/etc/ld.so.conf.d
install -m 0644 packaging/niova-core.conf \
    %{buildroot}/etc/ld.so.conf.d/niova-core.conf

# ---------------------------------------------------------------------------
# %post
# ---------------------------------------------------------------------------
%post
/sbin/ldconfig

# ---------------------------------------------------------------------------
# %postun
# ---------------------------------------------------------------------------
%postun
/sbin/ldconfig

# ---------------------------------------------------------------------------
# %files
# ---------------------------------------------------------------------------
%files

# Runtime libraries
%dir %{niova_prefix}
%dir %{niova_prefix}/lib
%{niova_prefix}/lib/*.so*

# Headers (for downstream builds)
%dir %{niova_prefix}/include
%{niova_prefix}/include/

# ldconfig drop-in
/etc/ld.so.conf.d/niova-core.conf

# ---------------------------------------------------------------------------
# %changelog
# ---------------------------------------------------------------------------
%changelog
* Tue Apr 08 2026 Niova Build System <build@niova.io> - 1.0.0-1
- Initial RPM release
- Bundles libbacktrace and niova-core C libraries
- aarch64 build
