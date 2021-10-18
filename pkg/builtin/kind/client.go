/*
Copyright 2021 The KubeVela Authors.

The code is inspired from the Kind repo at https://github.com/kubernetes-sigs/kind/blob/main/pkg/cmd/kind/load/docker-image/docker-image.go

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kind

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/klog/v2"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"
	"sigs.k8s.io/kind/pkg/errors"
	"sigs.k8s.io/kind/pkg/exec"
	"sigs.k8s.io/kind/pkg/fs"
)

func LoadDockerImage(imageName string) error {
	return LoadDockerImagesWithFlags([]string{imageName}, "", nil)
}

func LoadDockerImages(imageNames []string) error {
	return LoadDockerImagesWithFlags(imageNames, "", nil)
}

func LoadDockerImagesWithFlags(imageNames []string, flagName string, flagNodes []string) error {
	provider := cluster.NewProvider(
		cluster.ProviderWithDocker(),
	)

	// Set cluster context name by default
	if flagName == "" {
		flagName = cluster.DefaultName
	}

	// Check that the image exists locally and gets its ID, if not return error
	var imageIDs []string
	for _, imageName := range imageNames {
		imageID, err := imageID(imageName)
		if err != nil {
			return fmt.Errorf("image: %q not present locally", imageName)
		}
		imageIDs = append(imageIDs, imageID)
	}

	// Check if the cluster nodes exist
	nodeList, err := provider.ListInternalNodes(flagName)
	if err != nil {
		return err
	}
	if len(nodeList) == 0 {
		return fmt.Errorf("no nodes found for cluster %q", flagName)
	}

	// map cluster nodes by their name
	nodesByName := map[string]nodes.Node{}
	for _, node := range nodeList {
		// TODO(bentheelder): this depends on the fact that ListByCluster()
		// will have name for nameOrId.
		nodesByName[node.String()] = node
	}

	// pick only the user selected nodes and ensure they exist
	// the default is all nodes unless flags.Nodes is set
	candidateNodes := nodeList
	if len(flagNodes) > 0 {
		candidateNodes = []nodes.Node{}
		for _, name := range flagNodes {
			node, ok := nodesByName[name]
			if !ok {
				return fmt.Errorf("unknown node: %q", name)
			}
			candidateNodes = append(candidateNodes, node)
		}
	}

	// pick only the nodes that don't have the image
	selectedNodes := []nodes.Node{}
	fns := []func() error{}
	for i, imageName := range imageNames {
		imageID := imageIDs[i]
		for _, node := range candidateNodes {
			id, err := nodeutils.ImageID(node, imageName)
			if err != nil || id != imageID {
				selectedNodes = append(selectedNodes, node)
			}
		}
		if len(selectedNodes) == 0 {
			continue
		}
	}

	// Setup the tar path where the images will be saved
	dir, err := fs.TempDir("", "images-tar")
	if err != nil {
		return errors.Wrap(err, "failed to create tempdir")
	}
	defer os.RemoveAll(dir)
	imagesTarPath := filepath.Join(dir, "images.tar")
	// Save the images into a tar
	err = save(imageNames, imagesTarPath)
	if err != nil {
		return err
	}

	// Load the images on the selected nodes
	for _, selectedNode := range selectedNodes {
		selectedNode := selectedNode // capture loop variable
		fns = append(fns, func() error {
			return loadImage(imagesTarPath, selectedNode)
		})
	}
	return errors.UntilErrorConcurrent(fns)
}

// TODO: we should consider having a cluster method to load images

// loads an image tarball onto a node
func loadImage(imageTarName string, node nodes.Node) error {
	f, err := os.Open(imageTarName)
	if err != nil {
		return errors.Wrap(err, "failed to open image")
	}
	defer func() {
		if err := f.Close(); err != nil {
			klog.Error(err, "Failed to close file")
		}
	}()
	return nodeutils.LoadImageArchive(node, f)
}

// save saves images to dest, as in `docker save`
func save(images []string, dest string) error {
	commandArgs := append([]string{"save", "-o", dest}, images...)
	return exec.Command("docker", commandArgs...).Run()
}

// imageID return the ID of the container image
func imageID(containerNameOrID string) (string, error) {
	cmd := exec.Command("docker", "image", "inspect",
		"-f", "{{ .ID }}",
		containerNameOrID, // ... against the container
	)
	lines, err := exec.OutputLines(cmd)
	if err != nil {
		return "", err
	}
	if len(lines) != 1 {
		return "", errors.Errorf("Docker image ID should only be one line, got %d lines", len(lines))
	}
	return lines[0], nil
}
