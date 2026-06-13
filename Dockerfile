# Build either the manager or the scheduler: --build-arg CMD=manager|scheduler
FROM golang:1.26 AS builder
ARG CMD=manager
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/
COPY pkg/ pkg/
RUN CGO_ENABLED=0 GOOS=linux go build -a -o app ./cmd/${CMD}

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/app /app
USER 65532:65532
ENTRYPOINT ["/app"]
