# Stage 1: Build environment
# Using Fedora 42 which includes Go 1.24 required by our go.mod
FROM fedora:42 AS builder

# Build arguments for multi-platform support and versioning
ARG TARGETOS
ARG TARGETARCH
ARG RELEASE_VERSION
ENV RELEASE_VERSION=$RELEASE_VERSION

# Install build dependencies
# - golang: Required for building the application (Go 1.24.1 from Fedora 42)
# - libnbd-devel: Development files for NBD (Network Block Device) support
RUN dnf install -y golang libnbd-devel

# Set up build workspace
WORKDIR /workspace

# Copy pre-built binary from local build
# Note: The binary is built by 'make -C v2v-helper build' before docker build
# This is part of the VM migration toolchain for converting between different platforms
COPY manager manager

# Stage 2: Runtime environment
# Using same Fedora version for consistency and required runtime dependencies
FROM fedora:42

WORKDIR /tmp/rpms

RUN curl -L -o nbdkit.rpm https://github.com/platform9/vjailbreak/raw/refs/heads/main/assets/rpms/nbdkit-1.42.4-1.fc42.x86_64.rpm && \
    curl -L -o nbdkit-vddk.rpm https://github.com/platform9/vjailbreak/raw/refs/heads/main/assets/rpms/nbdkit-vddk-plugin-1.42.4-1.fc42.x86_64.rpm && \
    curl -L -o libnbd.rpm https://github.com/platform9/vjailbreak/raw/refs/heads/main/assets/rpms/libnbd-1.22.2-1.fc42.x86_64.rpm && \
    curl -L -o nbdkit-server.rpm https://github.com/platform9/vjailbreak/raw/refs/heads/main/assets/rpms/nbdkit-server-1.42.4-1.fc42.x86_64.rpm && \
    curl -L -o nbdkit-basic-plugins.rpm https://github.com/platform9/vjailbreak/raw/refs/heads/main/assets/rpms/nbdkit-basic-plugins-1.42.4-1.fc42.x86_64.rpm && \
    curl -L -o nbdkit-basic-filters.rpm https://github.com/platform9/vjailbreak/raw/refs/heads/main/assets/rpms/nbdkit-basic-filters-1.42.4-1.fc42.x86_64.rpm && \
    dnf install -y ./*.rpm && \
    dnf clean all && \
    rm -rf /tmp/rpms

    # Add version lock for all installed packages
RUN dnf versionlock add '*' && \
    dnf clean all && \
    rm -rf /var/cache/dnf

WORKDIR /tmp
RUN curl -L -o virt-v2v-2.7.13-1.fc42.x86_64.rpm https://github.com/platform9/vjailbreak/raw/refs/heads/main/assets/rpms/virt-v2v-2.7.13-1.fc42.x86_64.rpm

#RUN dnf repoquery --requires --resolve virt-v2v-2.7.13-1.fc42.x86_64.rpm


RUN dnf install -y --nobest --skip-broken /tmp/virt-v2v-2.7.13-1.fc42.x86_64.rpm && \
    dnf clean all && \
    rm -rf /var/cache/dnf

# Copy the binary from builder stage to the runtime environment
COPY --from=builder /workspace/manager /home/fedora/manager

