package node

//go:generate go tool counterfeiter -generate

// Mounter abstracts mount operations for testing.
//
//counterfeiter:generate -o nodefakes/fake_mounter.go --fake-name FakeMounter . Mounter
type Mounter interface {
	Mount(source, target, fstype string, flags uintptr, data string) error
	Unmount(target string, flags int) error
	IsMountPoint(path string) (bool, error)
}
