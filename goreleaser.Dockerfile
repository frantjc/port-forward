FROM gcr.io/distroless/static:nonroot
RUN apk add iptables
COPY manager /usr/local/bin
ENTRYPOINT ["/usr/local/bin/manager"]
