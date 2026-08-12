# syntax=docker/dockerfile:1@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89

ARG GO_IMAGE=golang:1.26-bookworm@sha256:6c5605ab3a9a9fb3c4eafe5b3d63cdbf3881caf113262b67862547b54a9db599
ARG DEBIAN_IMAGE=debian:bookworm-slim@sha256:abd67ffcfa541b485a3dff59865ab629aa048a6c613e639d36e7456b0b229241

FROM --platform=${BUILDPLATFORM} ${GO_IMAGE} AS build

ARG TARGETARCH
ARG TARGETOS

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod edit -dropreplace github.com/kubernetes-csi/csi-test \
    -dropreplace github.com/kubernetes-csi/csi-test/v5 && \
    go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build -trimpath -o /out/justmount ./main.go

FROM ${DEBIAN_IMAGE} AS glusterfs-source

ARG DEBIAN_SNAPSHOT=20260810T000000Z
ARG GLUSTERFS_DEBIAN_VERSION=10.3-5
ARG GLUSTERFS_PACKAGE_VERSION=10.3-5+justmount1

COPY build/glusterfs /opt/justmount-glusterfs
RUN chmod 0755 /opt/justmount-glusterfs/*.sh && \
    JUSTMOUNT_DEBIAN_SNAPSHOT="${DEBIAN_SNAPSHOT}" \
        /opt/justmount-glusterfs/configure-apt-snapshot.sh && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        dpkg-dev \
        patch && \
    JUSTMOUNT_DEBIAN_SNAPSHOT="${DEBIAN_SNAPSHOT}" \
    JUSTMOUNT_GLUSTERFS_DEBIAN_VERSION="${GLUSTERFS_DEBIAN_VERSION}" \
    JUSTMOUNT_GLUSTERFS_PACKAGE_VERSION="${GLUSTERFS_PACKAGE_VERSION}" \
        /opt/justmount-glusterfs/prepare-source.sh

FROM glusterfs-source AS glusterfs-packages

RUN JUSTMOUNT_DEBIAN_SNAPSHOT="${DEBIAN_SNAPSHOT}" \
    JUSTMOUNT_GLUSTERFS_PACKAGE_VERSION="${GLUSTERFS_PACKAGE_VERSION}" \
        /opt/justmount-glusterfs/build-debs.sh

FROM ${DEBIAN_IMAGE}

ARG DEBIAN_SNAPSHOT=20260810T000000Z
ARG GLUSTERFS_PACKAGE_VERSION=10.3-5+justmount1

COPY --chmod=0755 build/glusterfs/configure-apt-snapshot.sh /usr/local/sbin/configure-apt-snapshot
COPY --from=glusterfs-packages /out/ /tmp/glusterfs-debs/
RUN JUSTMOUNT_DEBIAN_SNAPSHOT="${DEBIAN_SNAPSHOT}" \
        /usr/local/sbin/configure-apt-snapshot && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
      ca-certificates \
      fuse3 \
      s3fs \
      sshfs \
      /tmp/glusterfs-debs/*.deb && \
    test "$(dpkg-query -W -f='${Version}' glusterfs-client)" = \
      "${GLUSTERFS_PACKAGE_VERSION}" && \
    glusterfs --help 2>&1 | grep -q -- '--fuse-setlk-handle-interrupt' && \
    rm -rf /var/lib/apt/lists/* /tmp/glusterfs-debs \
      /usr/local/sbin/configure-apt-snapshot
COPY --from=build /out/justmount /justmount
USER 0:0
ENTRYPOINT ["/justmount"]
