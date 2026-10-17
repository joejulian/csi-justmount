#!/bin/sh
set -eu

: "${JUSTMOUNT_GLUSTERFS_PACKAGE_VERSION:?JUSTMOUNT_GLUSTERFS_PACKAGE_VERSION must be set}"

readonly source_dir="/build/glusterfs-10.3"

mkdir -p /out

DEBIAN_FRONTEND=noninteractive apt-get install --yes --no-install-recommends \
    build-essential

apt-get build-dep --yes --no-install-recommends \
    -o APT::Get::Build-Dep-Automatic=true \
    -Pnocheck "${source_dir}"

cd "${source_dir}"
DEB_BUILD_OPTIONS="nocheck parallel=4" \
DEB_BUILD_PROFILES=nocheck \
SOURCE_DATE_EPOCH=1786320000 \
    dpkg-buildpackage --build=binary --no-sign

architecture="$(dpkg --print-architecture)"
readonly architecture
for package_name in \
    glusterfs-client \
    glusterfs-common \
    libgfapi0 \
    libgfchangelog0 \
    libgfrpc0 \
    libgfxdr0 \
    libglusterd0 \
    libglusterfs0
do
    package_path="/build/${package_name}_${JUSTMOUNT_GLUSTERFS_PACKAGE_VERSION}_${architecture}.deb"
    test -f "${package_path}"
    cp "${package_path}" /out/
done
