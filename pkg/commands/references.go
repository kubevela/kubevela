package commands

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

const (
	// SideBar file name for docsify
	SideBar = "_sidebar.md"
	// NavBar file name for docsify
	NavBar = "_navbar.md"
	// IndexHTML file name for docsify
	IndexHTML = "index.html"
	// README file name for docsify
	README = "README.md"
)

// Port is the port for reference docs website
const Port = ":18081"

// NewReferencesCommand shows the website which hosts the reference docs for workload types and trait types
func NewReferencesCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "reference",
		Aliases: []string{"ref", "ref-docs"},
		Short:   "Opens up the reference docs website for workload types and trait types",
		Long:    "Opens up the reference docs website for workload types and trait types",
		Example: `vela reference`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			return startReferenceDocsSite(ctx, c, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.Flags().StringP("svc", "s", "", "service name")
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func startReferenceDocsSite(ctx context.Context, c types.Args, ioStreams cmdutil.IOStreams) error {
	home, err := system.GetVelaHomeDir()
	if err != nil {
		return err
	}
	referenceHome := filepath.Join(home, "reference")

	definitionPath := filepath.Join(referenceHome, "capabilities")
	if _, err := os.Stat(definitionPath); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(definitionPath, 0750); err != nil {
			return err
		}
	}
	docsPath := filepath.Join(referenceHome, "docs")
	if _, err := os.Stat(docsPath); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(docsPath, 0750); err != nil {
			return err
		}
	}

	capabilities, _, err := plugins.SyncDefinitionToLocal(ctx, c, definitionPath)
	if err != nil {
		return err
	}

	if err := plugins.CreateMarkdown(capabilities, docsPath, plugins.ReferenceSourcePath); err != nil {
		return err
	}

	if err := generateSideBar(capabilities, docsPath); err != nil {
		return err
	}

	if err := generateNavBar(docsPath); err != nil {
		return err
	}

	if err := generateIndexHTML(docsPath); err != nil {
		return err
	}

	if err := generateREADME(capabilities, docsPath); err != nil {
		return err
	}

	server := &http.Server{
		Addr:         Port,
		Handler:      http.FileServer(http.Dir(docsPath)),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	server.SetKeepAlivesEnabled(true)
	errCh := make(chan error, 1)

	launch(server, errCh)

	select {
	case err = <-errCh:
		return err
	case <-time.After(time.Second):
		var url = "http://127.0.0.1" + Port
		if err := OpenBrowser(url); err != nil {
			ioStreams.Infof("automatically invoking browser failed: %v\nPlease visit %s for reference docs", err, url)
		}
	}

	// handle signal: SIGTERM(15)
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGTERM)

	<-sc
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func launch(server *http.Server, errChan chan<- error) {
	go func() {
		// http.Handle("/", http.FileServer(http.Dir(docsPath)))
		err := server.ListenAndServe()
		if err != nil && errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()
}

func generateSideBar(capabilities []types.Capability, docsPath string) error {
	sideBar := filepath.Join(docsPath, SideBar)
	workloads, traits := getWorkloadsAndTraits(capabilities)
	f, err := os.Create(sideBar)
	if err != nil {
		return err
	}
	if _, err := f.WriteString("- Workload Types\n"); err != nil {
		return nil
	}
	workloadFolderName := "workload-types"
	for _, w := range workloads {
		if _, err := f.WriteString(fmt.Sprintf("  - [%s](%s/%s.md)\n", w, workloadFolderName, w)); err != nil {
			return nil
		}
	}
	if _, err := f.WriteString("- Traits\n"); err != nil {
		return nil
	}
	traitFolderName := "traits"
	for _, t := range traits {
		if _, err := f.WriteString(fmt.Sprintf("  - [%s](%s/%s.md)\n", t, traitFolderName, t)); err != nil {
			return nil
		}
	}
	return nil
}

func generateNavBar(docsPath string) error {
	sideBar := filepath.Join(docsPath, NavBar)
	_, err := os.Create(sideBar)
	if err != nil {
		return err
	}
	return nil
}

func generateIndexHTML(docsPath string) error {
	indexHTML := `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>KubeVela Reference Docs</title>
  <meta http-equiv="X-UA-Compatible" content="IE=edge,chrome=1" />
  <meta name="description" content="Description">
  <meta name="viewport" content="width=device-width, initial-scale=1.0, minimum-scale=1.0">
  <link rel="stylesheet" href="//cdn.jsdelivr.net/npm/docsify@4/lib/themes/vue.css">
  <link rel="stylesheet" href="//cdn.jsdelivr.net/npm/docsify-sidebar-collapse/dist/sidebar.min.css" />
  <link rel="stylesheet" href="./resources/css/custom.css">
</head>
<body>
  <div id="app"></div>
  <script>
    window.$docsify = {
      name: 'KubeVela Reference Docs',
      loadSidebar: true,
      loadNavbar: true,
      subMaxLevel: 1,
      alias: {
        '/_sidebar.md': '/_sidebar.md',
        '/_navbar.md': '/_navbar.md'
      },
      formatUpdated: '{MM}/{DD}/{YYYY} {HH}:{mm}:{ss}',
    }
  </script>
  <!-- Docsify v4 -->
  <script src="//cdn.jsdelivr.net/npm/docsify@4"></script>
  <script src="//cdn.jsdelivr.net/npm/docsify/lib/docsify.min.js"></script>
  <!-- plugins -->
  <script src="//cdn.jsdelivr.net/npm/docsify-sidebar-collapse/dist/docsify-sidebar-collapse.min.js"></script>
</body>
</html>
`
	return ioutil.WriteFile(filepath.Join(docsPath, IndexHTML), []byte(indexHTML), 0600)
}

func generateREADME(capabilities []types.Capability, docsPath string) error {
	readmeMD := filepath.Join(docsPath, README)
	f, err := os.Create(readmeMD)
	if err != nil {
		return err
	}
	if _, err := f.WriteString("# KubeVela Reference Docs for Workload Types and Traits\n" +
		"Click the navigation bar on the left or the links below to look into the detailed referennce of a Workload type or a Trait.\n"); err != nil {
		return err
	}

	workloads, traits := getWorkloadsAndTraits(capabilities)

	if _, err := f.WriteString("## Workload Types\n"); err != nil {
		return err
	}
	workloadFolderName := "workload-types"
	for _, w := range workloads {
		if _, err := f.WriteString(fmt.Sprintf("  - [%s](%s/%s.md)\n", w, workloadFolderName, w)); err != nil {
			return err
		}
	}
	if _, err := f.WriteString("## Traits\n"); err != nil {
		return err
	}
	traitFolderName := "traits"
	for _, t := range traits {
		if _, err := f.WriteString(fmt.Sprintf("  - [%s](%s/%s.md)\n", t, traitFolderName, t)); err != nil {
			return err
		}
	}
	return nil
}

func getWorkloadsAndTraits(capabilities []types.Capability) ([]string, []string) {
	var workloads, traits []string
	for _, c := range capabilities {
		switch c.Type {
		case types.TypeWorkload:
			workloads = append(workloads, c.Name)
		case types.TypeTrait:
			traits = append(traits, c.Name)
		case types.TypeScope:

		}
	}
	return workloads, traits
}
