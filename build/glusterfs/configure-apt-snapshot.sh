#!/bin/sh
set -eu

: "${JUSTMOUNT_DEBIAN_SNAPSHOT:?JUSTMOUNT_DEBIAN_SNAPSHOT must be set}"

cat > /etc/apt/sources.list <<EOF
deb [check-valid-until=no] http://snapshot.debian.org/archive/debian/${JUSTMOUNT_DEBIAN_SNAPSHOT}/ bookworm main
deb-src [check-valid-until=no] http://snapshot.debian.org/archive/debian/${JUSTMOUNT_DEBIAN_SNAPSHOT}/ bookworm main
deb [check-valid-until=no] http://snapshot.debian.org/archive/debian/${JUSTMOUNT_DEBIAN_SNAPSHOT}/ bookworm-updates main
deb [check-valid-until=no] http://snapshot.debian.org/archive/debian-security/${JUSTMOUNT_DEBIAN_SNAPSHOT}/ bookworm-security main
EOF

rm -f /etc/apt/sources.list.d/debian.sources
