package main

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func main() {
	// Set up the kubeconfig file path.
	kubeconfig := "../.kube/config" // Update the path to your kubeconfig file.

	// Set the namespace.
	namespace := "vela-system" // Replace with your desired namespace.


	// Build the configuration from the kubeconfig file.
	cfg, err := config.GetConfigWithContext("kubeconfig")
	if err != nil {
		panic(err)
		return
	}

	// Create a client using the configuration.
	c, err := client.New(cfg, client.Options{})
	if err != nil {
		panic(err)
		return
	}

	// Set the ComponentDefinition list.
	componentDefinitionList := &v1beta1.ComponentDefinitionList{}


	// Retrieve the ComponentDefinition resources using the client and ListOptions.
	err = c.List(context.TODO(), componentDefinitionList, client.InNamespace("vela-system"))
	if err != nil {
		panic(err)
		return
	}

	// Print the retrieved ComponentDefinition resources.
	for _, cd := range componentDefinitionList.Items {
		fmt.Printf("ComponentDefinition: %s\n", cd.Name)
	}
}
