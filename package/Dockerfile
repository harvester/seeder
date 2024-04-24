# syntax=docker/dockerfile:1.7.0

# Build the manager binary
FROM registry.suse.com/bci/bci-base:15.5
RUN zypper -n install ipmitool=1.8.18.238.gb7adc1d-150400.3.6.1 && zypper clean
ARG TARGETPLATFORM

RUN if [ "$TARGETPLATFORM" != "linux/amd64" ] && [ "$TARGETPLATFORM" != "linux/arm64" ]; then \
    echo "Error: Unsupported TARGETPLATFORM: $TARGETPLATFORM" && \
    exit 1; \
    fi

ENV ARCH=${TARGETPLATFORM#linux/}
WORKDIR /
COPY bin/manager-${ARCH} /bin/manager
USER 65532:65532

ENTRYPOINT ["/bin/manager"]
