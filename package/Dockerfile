# Build the manager binary
FROM registry.suse.com/bci/bci-base:15.4
RUN zypper -n install ipmitool
WORKDIR /
COPY bin/manager /bin/manager
USER 65532:65532

ENTRYPOINT ["/bin/manager"]
