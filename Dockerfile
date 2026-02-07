FROM golang:1.24-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod edit -dropreplace github.com/kubernetes-csi/csi-test \
    -dropreplace github.com/kubernetes-csi/csi-test/v5 && \
    go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/justmount ./main.go

FROM debian:bookworm-slim
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      ca-certificates \
      ceph-fuse \
      fuse3 \
      glusterfs-client \
      s3fs \
      sshfs && \
    rm -rf /var/lib/apt/lists/*
COPY --from=build /out/justmount /justmount
USER 0:0
ENTRYPOINT ["/justmount"]
