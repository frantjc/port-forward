FROM golang:1.23 AS build
ARG GOOS=linux
ARG GOARCH

WORKDIR $GOPATH/src/github.com/frantjc/port-forward
COPY go.mod go.sum ./
RUN go mod download
COPY main.go version.go ./
COPY api/ api/
COPY controllers/ controllers/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -o /portfwd ./cmd/portfwd

FROM alpine
WORKDIR /
RUN apk add iptables
COPY --from=build /portfwd /usr/local/bin
ENTRYPOINT ["/usr/local/bin/portfwd"]
