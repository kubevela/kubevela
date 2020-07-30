package test

import (
	"os"
	"os/exec"
	"path"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	rudrPath, _ = os.Getwd()
)

func createKubernetesClient() (client.Client, error) {
	c, err := client.New(config.GetConfigOrDie(), client.Options{})

	return c, err
}

func TestCreateKubernetesClient(t *testing.T) {
	_, err := createKubernetesClient()
	if err != nil {
		t.Errorf("Failed to create a Kubernetes client: %s", err)
	}
}

// TestBuildCliBinary is to build rudr binary.
func TestBuildCliBinary(t *testing.T) {
	rudrPath, err := os.Getwd()
	mainPath := path.Join(rudrPath, "../../cmd/rudrx/main.go")
	if err != nil {
		t.Errorf("Failed to build rudr binary: %s", err)
	}

	cmd := exec.Command("go", "build", "-o", path.Join(rudrPath, "rudr"), mainPath)

	stdout, err := cmd.Output()
	if err != nil {
		t.Errorf("Failed to build rudr binary: %s", err)
	}
	t.Log(stdout, err)

	// TODO(zzxwill) If this failed, all other test-cases should be terminated

}

func Command(name string, arg ...string) *exec.Cmd {
	commandName := path.Join(rudrPath, name)
	return exec.Command(commandName, arg...)
}

func TestTraitsList(t *testing.T) {
	cmd := Command("rudr", []string{"traits", "list"}...)
	stdout, err := cmd.Output()
	t.Log(string(stdout), err)
	if err != nil {
		t.Errorf("Failed to list traits: %s", err)
	}

}
