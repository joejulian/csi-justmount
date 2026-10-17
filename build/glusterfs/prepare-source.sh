#!/bin/sh
set -eu

: "${JUSTMOUNT_DEBIAN_SNAPSHOT:?JUSTMOUNT_DEBIAN_SNAPSHOT must be set}"
: "${JUSTMOUNT_GLUSTERFS_DEBIAN_VERSION:?JUSTMOUNT_GLUSTERFS_DEBIAN_VERSION must be set}"
: "${JUSTMOUNT_GLUSTERFS_PACKAGE_VERSION:?JUSTMOUNT_GLUSTERFS_PACKAGE_VERSION must be set}"

readonly source_package="glusterfs_${JUSTMOUNT_GLUSTERFS_DEBIAN_VERSION}"
readonly source_dir="/build/glusterfs-10.3"
readonly source_url="https://snapshot.debian.org/archive/debian/${JUSTMOUNT_DEBIAN_SNAPSHOT}/pool/main/g/glusterfs"

mkdir -p /build
cd /build

curl --fail --location --show-error --silent \
    --output "${source_package}.dsc" \
    "${source_url}/${source_package}.dsc"
curl --fail --location --show-error --silent \
    --output glusterfs_10.3.orig.tar.gz \
    "${source_url}/glusterfs_10.3.orig.tar.gz"
curl --fail --location --show-error --silent \
    --output "${source_package}.debian.tar.xz" \
    "${source_url}/${source_package}.debian.tar.xz"

sha256sum --check /opt/justmount-glusterfs/source.SHA256SUMS
dpkg-source --extract "${source_package}.dsc" "${source_dir}"

(
    cd /opt/justmount-glusterfs/patches
    sha256sum --check SHA256SUMS
)

cd "${source_dir}"
while IFS= read -r patch_name; do
    patch --batch --forward --fuzz=0 --strip=1 \
        < "/opt/justmount-glusterfs/patches/${patch_name}"
done < /opt/justmount-glusterfs/patches/series

/opt/justmount-glusterfs/verify-source.sh "${source_dir}"

{
    cat <<EOF
glusterfs (${JUSTMOUNT_GLUSTERFS_PACKAGE_VERSION}) bookworm; urgency=medium

  * Backport GlusterFS FUSE/RPC lock timeout and reconnect fixes.

 -- Justmount image build <noreply@example.invalid>  Mon, 10 Aug 2026 00:00:00 +0000

EOF
    cat debian/changelog
} > debian/changelog.justmount
mv debian/changelog.justmount debian/changelog
