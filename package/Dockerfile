# Build the manager binary
FROM registry.suse.com/bci/bci-base:15.5
RUN zypper -n install ipmitool=1.8.18.238.gb7adc1d-150400.3.6.1 && zypper clean
WORKDIR /
COPY bin/manager /bin/manager
USER 65532:65532

ENTRYPOINT ["/bin/manager"]
