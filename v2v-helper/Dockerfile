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

# Add virtio-win repository for Windows drivers
# Required for VM migration and conversion support
# This provides Windows VirtIO drivers needed during VM conversion
ADD https://fedorapeople.org/groups/virt/virtio-win/virtio-win.repo /etc/yum.repos.d/virtio-win.repo

# Install runtime dependencies:
# - nbdkit: Network Block Device toolkit for accessing disk images
# - nbdkit-vddk-plugin: VMware VDDK plugin for accessing VMware disks
# - libnbd: Network Block Device client library
# - virt-v2v: Convert VMs between platforms (e.g., VMware to OpenStack)
# - virtio-win: VirtIO drivers for Windows guests
RUN dnf install -y \
    nbdkit \
    nbdkit-vddk-plugin \
    libnbd \
    virt-v2v-2.7.13 \
    virtio-win && \
    dnf clean all && \
    rm -rf /var/cache/dnf

# Copy the binary from builder stage to the runtime environment
COPY --from=builder /workspace/manager /home/fedora/manager
