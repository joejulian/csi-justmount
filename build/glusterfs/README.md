# Patched GlusterFS client packages

The Justmount image builds its GlusterFS client packages from Debian's exact
`glusterfs 10.3-5` source package rather than installing the unpatched binary
package. The base image, Debian repository snapshot, source files, and local
patches are all pinned or checksummed so amd64 and arm64 releases consume the
same inputs.

The source corresponds to upstream GlusterFS `v10.3` commit
`d1c74321570e3ec77c1d91293549900f4605d613`. The behavioral backports are
exported directly from commits `0c197471ead2461d3df350a092e3fbec38cb4226` and
`9600359667df534012683a73449f9b17397f42a9`. The patches add:

- upstream `--fuse-setlk-handle-interrupt` support from commit `2f1066d5c2`,
  with its safer default of `false`;
- recognition of version 400 FOP lock requests so blocking locks do not enter
  the ordinary frame-timeout queue;
- ordinary-request unwind before lock-request unwind, preventing a FUSE lock
  interrupt from synchronously waiting on a callback queued behind it; and
- non-fatal handling of late replies whose timed-out XID is no longer present.

A packaging-only patch disables GlusterFS link-time optimization. Debian's
other hardening flags remain enabled; avoiding the serial LTO link substantially
reduces both native and QEMU-emulated package-build time.

`prepare-source.sh` verifies every downloaded and vendored input before
applying the patches. `verify-source.sh` then checks focused source markers.
The final image build verifies the patched Debian package version and the new
GlusterFS command-line option. The GoReleaser image build compiles the packages
independently for each configured target architecture.
