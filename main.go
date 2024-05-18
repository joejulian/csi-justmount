package main

import (
	"log"
	"os"

	"github.com/joejulian/csi-justmount/pkg/driver"
)

func main() {
	endpoint := "unix:///var/lib/kubelet/plugins/justmount/csi.sock"
	if e := os.Getenv("CSI_ENDPOINT"); e != "" {
		endpoint = e
	}

	d := driver.NewJustMountDriver(endpoint)
	if err := d.Run(); err != nil {
		log.Fatalf("Failed to run JustMount driver: %v", err)
	}
}
