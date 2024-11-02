package main

import (
	"log"

	"github.com/joejulian/csi-justmount/pkg/controller"
	"github.com/joejulian/csi-justmount/pkg/node"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func main() {
	// Define command-line flags for each service
	pflag.String("controller-endpoint", "/tmp/csi-controller.sock", "CSI Controller service endpoint")
	pflag.String("node-endpoint", "/tmp/csi-node.sock", "CSI Node service endpoint")
	pflag.String("nodeID", "example-node-id", "Unique identifier for the node")
	pflag.Parse()

	// Bind flags to Viper
	viper.BindPFlags(pflag.CommandLine)

	// Read values from Viper
	controllerEndpoint := viper.GetString("controller-endpoint")
	nodeEndpoint := viper.GetString("node-endpoint")
	nodeID := viper.GetString("nodeID")

	// Initialize and run the Controller
	controllerService := controller.NewController(controllerEndpoint, false)
	if err := controllerService.Run(); err != nil {
		log.Fatalf("Failed to run Controller: %v", err)
	}

	// Initialize and run the Node service
	nodeService := node.NewNode(nodeEndpoint, nodeID)
	if err := nodeService.Run(); err != nil {
		log.Fatalf("Failed to run Node service: %v", err)
	}
}
