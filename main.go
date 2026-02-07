package main

import (
	"log"

	"github.com/joejulian/csi-justmount/pkg/node"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func main() {
	// Define command-line flags for the node service
	pflag.String("node-endpoint", "/tmp/csi-node.sock", "CSI Node service endpoint")
	pflag.String("node-id", "example-node-id", "Unique identifier for the node")
	pflag.Parse()

	// Bind flags to Viper
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		log.Fatalf("Failed to bind flags: %v", err)
	}

	// Read values from Viper
	nodeEndpoint := viper.GetString("node-endpoint")
	nodeID := viper.GetString("node-id")

	// Initialize and run the Node service
	nodeService := node.NewNode(nodeID, nodeEndpoint)
	if err := nodeService.Run(); err != nil {
		log.Fatalf("Failed to run Node service: %v", err)
	}
}
