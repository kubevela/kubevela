/*
Copyright 2021 The KubeVela Authors.

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

package cli

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	innerVersion "github.com/oam-dev/kubevela/version"
)

var (
	cArgs              CtrlPlaneArgs
	k3sDownloadTmpl    = "https://%s//k3s-io/k3s/releases/download/%s/k3s"
	k3sVersion         = "v1.21.10+k3s1"
	k3sBinaryLocation  = "/usr/local/bin/k3s"
	kubeConfigLocation = "/etc/rancher/k3s/k3s.yaml"

	info func(a ...interface{})
	errf func(format string, a ...interface{})
)

// CtrlPlaneArgs defines arguments for ctrl-plane command
type CtrlPlaneArgs struct {
	TLSSan                    string
	DBEndpoint                string
	IsMainland                bool
	IsJoin                    bool
	Token                     string
	DisableWorkloadController bool
	// InstallVelaParam is parameters passed to vela install command
	// e.g. "--detail --version=x.y.z"
	InstallVelaParam string
}

// NewCtrlPlaneCommand create ctrl-plane command
func NewCtrlPlaneCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ctrl-plane",
		Short: "Quickly setup a KubeVela control plane",
		Long:  "Quickly setup a KubeVela control plane, using K3s and only for Linux now",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.AddCommand(
		NewInstallCmd(c, ioStreams),
		NewKubeConfigCmd(),
		NewUninstallCmd(),
	)
	return cmd
}

// NewInstallCmd create install cmd
func NewInstallCmd(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	info = ioStreams.Info
	errf = ioStreams.Errorf
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Quickly setup a KubeVela control plane",
		Long:  "Quickly setup a KubeVela control plane, using K3s and only for Linux now",
		Run: func(cmd *cobra.Command, args []string) {
			if runtime.GOOS != "linux" {
				info("Launch control plane is not supported now in non-linux OS, exiting")
				return
			}
			defer func() {
				err := cleanup()
				if err != nil {
					errf("Fail to clean up install script: %v", err)
				}
			}()

			// Step.1 Set up K3s as control plane cluster
			err := SetupK3s(cmd.Context(), cArgs)
			if err != nil {
				errf("Fail to setup k3s: %v\n", err)
				return
			}
			info("Successfully setup cluster")

			// Step.2 Set KUBECONFIG
			err = os.Setenv("KUBECONFIG", kubeConfigLocation)
			if err != nil {
				errf("Fail to set KUBECONFIG environment var: %v\n", err)
				return
			}

			// Step.3 install vela-core
			if !cArgs.IsJoin {
				installCmd := NewInstallCommand(c, "1", ioStreams)
				installArgs := strings.Split(cArgs.InstallVelaParam, " ")
				if cArgs.DisableWorkloadController {
					installArgs = append(installArgs, "--set", "podOnly=true")
				}
				installCmd.SetArgs(installArgs)
				err = installCmd.Execute()
				if err != nil {
					errf("Fail to install vela-core in control plane: %v. You can try \"vela install\" later\n", err)
					return
				}
			}

			info("Cleaning up script...")
			info("Successfully set up KubeVela control plane, run: export KUBECONFIG=$(vela ctrl-plane kubeconfig) to access it")
		},
	}
	cmd.Flags().BoolVar(&cArgs.IsMainland, "mainland", false, "If set, use some mirror site to avoid network problem")
	cmd.Flags().BoolVar(&cArgs.IsJoin, "join", false, "If set, vela-core won't be installed again")
	cmd.Flags().StringVar(&cArgs.DBEndpoint, "database-endpoint", "", "Use an external database to store control plane metadata")
	cmd.Flags().StringVar(&cArgs.TLSSan, "tls-san", "", "Add additional hostname or IP as a Subject Alternative Name in the TLS cert")
	cmd.Flags().StringVar(&cArgs.Token, "token", "", "Token for register other node")
	cmd.Flags().BoolVar(&cArgs.DisableWorkloadController, "disable-workload-controller", true, "Disable controllers for Deployment/Job/ReplicaSet/StatefulSet/CronJob/DaemonSet")
	cmd.Flags().StringVar(&cArgs.InstallVelaParam, "install-param", innerVersion.VelaVersion, "Specify the parameters passed to vela install command")
	return cmd
}

func cleanup() error {
	files, err := filepath.Glob("/var/k3s-setup-*.sh")
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	return nil
}

// SetupK3s will set up K3s as control plane.
func SetupK3s(ctx context.Context, cArgs CtrlPlaneArgs) error {
	info("Downloading cluster setup script...")
	script, err := DownloadK3sScript(ctx)
	if err != nil {
		return errors.Wrap(err, "fail to download k3s setup script")
	}
	defer func(name string) {
		info("Cleaning up cluster setup script")
		err := os.Remove(name)
		if err != nil {
			fmt.Printf("Fail to delete install script %s: %v\n", name, err)
		}
	}(script)

	info("Downloading k3s binary...")
	err = PrepareK3sBin(ctx, cArgs.IsMainland)
	if err != nil {
		return errors.Wrap(err, "Fail to download k3s binary")
	}

	info("Setting up cluster...")
	args := []string{script}
	other := composeArgs(cArgs)
	args = append(args, other...)
	/* #nosec */
	cmd := exec.Command("/bin/sh", args...)

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "INSTALL_K3S_SKIP_DOWNLOAD=true")
	output, err := cmd.CombinedOutput()
	fmt.Print(string(output))
	return errors.Wrap(err, "K3s install script failed")
}

// composeArgs convert args from command to ones passed to k3s install script
func composeArgs(args CtrlPlaneArgs) []string {
	var shellArgs []string
	if args.DBEndpoint != "" {
		shellArgs = append(shellArgs, "--datastore-endpoint="+args.DBEndpoint)
	}
	if args.TLSSan != "" {
		shellArgs = append(shellArgs, "--tls-san="+args.TLSSan)
	}
	if args.Token != "" {
		shellArgs = append(shellArgs, "--token="+args.Token)
	}
	if args.DisableWorkloadController {
		shellArgs = append(shellArgs, "--kube-controller-manager-arg=controllers=*,-deployment,-job,-replicaset,-daemonset,-statefulset,-cronjob",
			// Traefik use Job to install, which is impossible without Job Controller
			"--disable", "traefik")
	}
	return shellArgs
}

// DownloadK3sScript download k3s install script
func DownloadK3sScript(ctx context.Context) (string, error) {
	scriptFile, err := ioutil.TempFile("/var", "k3s-setup-*.sh")
	if err != nil {
		return "", err
	}
	defer CloseQuietly(scriptFile)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://get.k3s.io", nil)
	if err != nil {
		return "", err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer CloseQuietly(res.Body)
	_, err = io.Copy(scriptFile, res.Body)
	if err != nil {
		return "", err
	}
	return scriptFile.Name(), nil
}

// PrepareK3sBin download k3s bin
func PrepareK3sBin(ctx context.Context, isMainland bool) error {
	downloadSite := "github.com"
	if isMainland {
		downloadSite = "hub.fastgit.xyz"
	}
	downloadURL := fmt.Sprintf(k3sDownloadTmpl, downloadSite, k3sVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer CloseQuietly(res.Body)
	/* #nosec */
	bin, err := os.OpenFile(k3sBinaryLocation, os.O_CREATE|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	defer CloseQuietly(bin)
	_, err = io.Copy(bin, res.Body)
	if err != nil {
		return err
	}
	info("Successfully downloading k3s binary to " + k3sBinaryLocation)
	return nil
}

// NewKubeConfigCmd create kubeconfig command for ctrl-plane
func NewKubeConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kubeconfig",
		Short: "print kubeconfig to access control plane",
		Run: func(cmd *cobra.Command, args []string) {
			_, err := os.Stat(kubeConfigLocation)
			if err != nil {
				return
			}
			fmt.Println(kubeConfigLocation)
		},
	}
	return cmd
}

// NewUninstallCmd create uninstall command
func NewUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "uninstall control plane",
		RunE: func(cmd *cobra.Command, args []string) error {
			// #nosec
			uninstallCmd := exec.Command("/usr/local/bin/k3s-uninstall.sh")
			return uninstallCmd.Run()
		},
	}
	return cmd
}

// CloseQuietly closes `io.Closer` quietly. Very handy and helpful for code
// quality too.
func CloseQuietly(d io.Closer) {
	_ = d.Close()
}
