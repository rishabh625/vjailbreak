# Multi-stage build for optimized image size
FROM ubuntu:22.04 as builder

# Install build dependencies
RUN apt-get update && apt-get install -y \
    wget \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Download and prepare any additional tools that need compilation
# (This stage can be used for building custom tools if needed)

# Main runtime image
FROM ubuntu:22.04

LABEL maintainer="vJailbreak Team <support@traycer.ai>"
LABEL description="v2v-helper container with RDM migration support"

# Set environment variables
ENV DEBIAN_FRONTEND=noninteractive
ENV PATH="/usr/local/bin:$PATH"

# Install system packages and RDM migration tools
RUN apt-get update && apt-get install -y \
    # Basic system tools
    bash \
    coreutils \
    util-linux \
    procps \
    # Device management and discovery tools
    lsblk \
    lsscsi \
    sg3-utils \
    udev \
    kmod \
    # Data copying and monitoring tools
    pv \
    rsync \
    dd \
    # Checksum and validation tools
    coreutils \
    # Network and storage tools
    nbd-client \
    open-iscsi \
    multipath-tools \
    # System monitoring and debugging
    strace \
    ltrace \
    iostat \
    iotop \
    # JSON processing for API interactions
    jq \
    # Network tools
    curl \
    wget \
    netcat-openbsd \
    # Compression tools
    gzip \
    bzip2 \
    xz-utils \
    # File system tools
    e2fsprogs \
    xfsprogs \
    dosfstools \
    # Python for scripting
    python3 \
    python3-pip \
    # SSL certificates
    ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

# Install Python packages for OpenStack integration
RUN pip3 install --no-cache-dir \
    python-cinderclient \
    python-novaclient \
    python-keystoneclient \
    requests \
    pyyaml

# Create necessary directories
RUN mkdir -p /app \
    /var/log/v2v-helper \
    /tmp/rdm-migration \
    /dev/mapper

# Copy application binaries (these would be built in CI/CD)
COPY --from=builder /usr/local/bin/* /usr/local/bin/ 2>/dev/null || true
COPY v2v-helper /app/
COPY scripts/ /app/scripts/

# Set up device access permissions
RUN groupadd -r storage && \
    useradd -r -g storage -s /bin/bash v2v-helper && \
    usermod -a -G disk,storage v2v-helper

# Create wrapper script for device access
RUN cat > /app/device-wrapper.sh << 'EOF'
#!/bin/bash
# Wrapper script to ensure proper device access for RDM operations

# Scan for new devices
echo "Scanning for storage devices..."
/usr/bin/rescan-scsi-bus.sh -a 2>/dev/null || true
udevadm settle

# List available block devices for debugging
echo "Available block devices:"
lsblk -o NAME,SIZE,TYPE,MOUNTPOINT,UUID

# List SCSI devices
echo "SCSI devices:"
lsscsi -v 2>/dev/null || true

# Execute the main application
exec "$@"
EOF

RUN chmod +x /app/device-wrapper.sh

# Create health check script
RUN cat > /app/healthcheck.sh << 'EOF'
#!/bin/bash
# Health check for v2v-helper with RDM support

# Check if main process is running
if ! pgrep -f "v2v-helper" > /dev/null; then
    echo "v2v-helper process not found"
    exit 1
fi

# Check device access
if ! ls /dev/sd* > /dev/null 2>&1 && ! ls /dev/nvme* > /dev/null 2>&1; then
    echo "No block devices accessible"
    exit 1
fi

# Check required tools
for tool in lsblk lsscsi pv dd; do
    if ! command -v "$tool" > /dev/null; then
        echo "Required tool $tool not found"
        exit 1
    fi
done

echo "Health check passed"
exit 0
EOF

RUN chmod +x /app/healthcheck.sh

# Set working directory
WORKDIR /app

# Set proper ownership
RUN chown -R v2v-helper:storage /app /var/log/v2v-helper /tmp/rdm-migration

# Switch to non-root user for security
USER v2v-helper

# Expose any necessary ports (adjust as needed)
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD /app/healthcheck.sh

# Set capabilities and security options in deployment manifests:
# securityContext:
#   capabilities:
#     add:
#       - SYS_ADMIN
#       - SYS_RAWIO
#       - MKNOD
#   privileged: false
#   allowPrivilegeEscalation: true

# Default command
ENTRYPOINT ["/app/device-wrapper.sh"]
CMD ["/app/v2v-helper"]