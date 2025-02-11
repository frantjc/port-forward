FROM alpine
RUN apk add iptables
ENTRYPOINT ["/usr/local/bin/portfwd"]
COPY portfwd /usr/local/bin
