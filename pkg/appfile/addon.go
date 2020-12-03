package appfile

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// Terraform is an addon
const Terraform = "terraform"

// Addon type, like `terraform`
type Addon map[string]interface{}

// DeployAddon deploys addon resources
func DeployAddon(addons map[string]Addon) error {
	for k, v := range addons {
		switch k {
		case Terraform:
			fmt.Printf(k, v)
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			path := v["path"]
			if err := os.Chdir(path.(string)); err != nil {
				return err
			}
			var cmd *exec.Cmd
			cmd = exec.Command("bash", "-c", "terraform init")
			if err := common.RealtimePrintCommandOutput(cmd); err != nil {
				return err
			}

			cmd = exec.Command("bash", "-c", "terraform apply --auto-approve")
			if err := common.RealtimePrintCommandOutput(cmd); err != nil {
				return err
			}
			if err := os.Chdir(cwd); err != nil {
				return err
			}
		default:

		}
	}
	return nil
}
