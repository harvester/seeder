FROM dmtf/redfish-mockup-server:latest
COPY mockup /mockup
COPY gencert.sh /mockup/gencert.sh
RUN chmod +x /mockup/gencert.sh && \
    /mockup/gencert.sh