FROM golang:1.22 as build
ARG GOOS
ARG GOARCH

WORKDIR $GOPATH/src/github.com/frantjc/port-forward
COPY go.mod go.sum ./
RUN go mod download
COPY main.go version.go ./
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=${GOOS:-linux} GOARCH=${GOARCH} go build -a -o /manager main.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /manager .
USER 65532:65532
ENTRYPOINT ["/manager"]
