#!/bin/sh
set -eu

readonly source_dir="${1:?usage: verify-source.sh SOURCE_DIR}"
readonly rpc_source="${source_dir}/rpc/rpc-lib/src/rpc-clnt.c"
readonly fuse_source="${source_dir}/xlators/mount/fuse/src/fuse-bridge.c"
readonly debian_rules="${source_dir}/debian/rules"

grep -Fq 'SFRAME_GET_PROGVER(sframe) == GLUSTER_FOP_VERSION_v2' "${rpc_source}"
grep -Fq 'list_append_init(&saved_frames->lk_sf.list,' "${rpc_source}"
grep -Fq 'reply for unknown or expired xid' "${rpc_source}"
grep -Fq 'setlk_handle_interrupt' "${fuse_source}"
grep -Fq '.default_value = "false"' "${fuse_source}"
grep -Fq -- '--disable-lto' "${debian_rules}"
