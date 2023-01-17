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
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/kubevela/workflow/pkg/cue/packages"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/docgen"
)

const (
	// SideBar file name for docsify
	SideBar = "_sidebar.md"
	// NavBar file name for docsify
	NavBar = "_navbar.md"
	// IndexHTML file name for docsify
	IndexHTML = "index.html"
	// CSS file name for custom CSS
	CSS = "custom.css"
	// README file name for docsify
	README = "README.md"
)

const (
	// Port is the port for reference docs website
	Port = ":18081"
)

var webSite bool
var generateDocOnly bool
var showFormat string

// NewCapabilityShowCommand shows the reference doc for a component type or trait
func NewCapabilityShowCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var revision, path, location, i18nPath string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show the reference doc for a component, trait, policy or workflow.",
		Long:  "Show the reference doc for component, trait, policy or workflow types. 'vela show' equals with 'vela def show'. ",
		Example: `0. Run 'vela show' directly to start a web server for all reference docs.  
1. Generate documentation for ComponentDefinition webservice:
> vela show webservice -n vela-system
2. Generate documentation for local CUE Definition file webservice.cue:
> vela show webservice.cue
3. Generate documentation for local Cloud Resource Definition YAML alibaba-vpc.yaml:
> vela show alibaba-vpc.yaml
4. Specify output format, markdown supported:
> vela show webservice --format markdown
5. Specify a language for output, by default, it's english. You can also load your own translation script:
> vela show webservice --location zh
> vela show webservice --location zh --i18n https://kubevela.io/reference-i18n.json
6. Show doc for a specified revision, it must exist in control plane cluster:
> vela show webservice --revision v1
7. Generate docs for all capabilities into folder $HOME/.vela/reference/docs/
> vela show
8. Generate all docs and start a doc server
> vela show --web
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			var capabilityName string
			if len(args) > 0 {
				capabilityName = args[0]
			} else if !webSite {
				cmd.Println("generating all capability docs into folder '~/.vela/reference/docs/', use '--web' to start a server for browser.")
				generateDocOnly = true
			}
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			var ver int
			if revision != "" {
				// v1, 1, both need to work
				version := strings.TrimPrefix(revision, "v")
				ver, err = strconv.Atoi(version)
				if err != nil {
					return fmt.Errorf("invalid revision: %w", err)
				}
			}
			if webSite || generateDocOnly {
				return startReferenceDocsSite(ctx, namespace, c, ioStreams, capabilityName)
			}
			if path != "" || showFormat == "md" || showFormat == "markdown" {
				return ShowReferenceMarkdown(ctx, c, ioStreams, capabilityName, path, location, i18nPath, namespace, int64(ver))
			}
			return ShowReferenceConsole(ctx, c, ioStreams, capabilityName, namespace, location, i18nPath, int64(ver))
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}

	cmd.Flags().BoolVarP(&webSite, "web", "", false, "start web doc site")
	cmd.Flags().StringVarP(&showFormat, "format", "", "", "specify format of output data, by default it's a pretty human readable format, you can specify markdown(md)")
	cmd.Flags().StringVarP(&revision, "revision", "r", "", "Get the specified revision of a definition. Use def get to list revisions.")
	cmd.Flags().StringVarP(&path, "path", "p", "", "Specify the path for of the doc generated from definition.")
	cmd.Flags().StringVarP(&location, "location", "l", "", "specify the location for of the doc generated from definition, now supported options 'zh', 'en'. ")
	cmd.Flags().StringVarP(&i18nPath, "i18n", "", "https://kubevela.io/reference-i18n.json", "specify the location for of the doc generated from definition, now supported options 'zh', 'en'. ")

	addNamespaceAndEnvArg(cmd)
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func generateWebsiteDocs(capabilities []types.Capability, docsPath string) error {
	if err := generateSideBar(capabilities, docsPath); err != nil {
		return err
	}

	if err := generateNavBar(docsPath); err != nil {
		return err
	}

	if err := generateIndexHTML(docsPath); err != nil {
		return err
	}
	if err := generateCustomCSS(docsPath); err != nil {
		return err
	}

	if err := generateREADME(capabilities, docsPath); err != nil {
		return err
	}
	return nil
}

func startReferenceDocsSite(ctx context.Context, ns string, c common.Args, ioStreams cmdutil.IOStreams, capabilityName string) error {
	home, err := system.GetVelaHomeDir()
	if err != nil {
		return err
	}
	referenceHome := filepath.Join(home, "reference")

	docsPath := filepath.Join(referenceHome, "docs")
	if _, err := os.Stat(docsPath); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(docsPath, 0750); err != nil {
			return err
		}
	}
	capabilities, err := docgen.GetNamespacedCapabilitiesFromCluster(ctx, ns, c, nil)
	if err != nil {
		return err
	}
	// check whether input capability is valid
	var capabilityIsValid bool
	var capabilityType types.CapType
	for _, c := range capabilities {
		if capabilityName == "" {
			capabilityIsValid = true
			break
		}
		if c.Name == capabilityName {
			capabilityIsValid = true
			capabilityType = c.Type
			break
		}
	}
	if !capabilityIsValid {
		return fmt.Errorf("%s is not a valid component, trait, policy or workflow", capabilityName)
	}

	cli, err := c.GetClient()
	if err != nil {
		return err
	}
	config, err := c.GetConfig()
	if err != nil {
		return err
	}
	pd, err := packages.NewPackageDiscover(config)
	if err != nil {
		return err
	}
	dm, err := c.GetDiscoveryMapper()
	if err != nil {
		return err
	}
	ref := &docgen.MarkdownReference{
		ParseReference: docgen.ParseReference{
			Client: cli,
			I18N:   &docgen.En,
		},
		DiscoveryMapper: dm,
	}

	if err := ref.CreateMarkdown(ctx, capabilities, docsPath, true, pd); err != nil {
		return err
	}

	if err = generateWebsiteDocs(capabilities, docsPath); err != nil {
		return err
	}

	if generateDocOnly {
		return nil
	}

	if capabilityType != types.TypeWorkload && capabilityType != types.TypeTrait && capabilityType != types.TypeScope &&
		capabilityType != types.TypeComponentDefinition && capabilityType != types.TypeWorkflowStep && capabilityType != "" {
		return fmt.Errorf("unsupported type: %v", capabilityType)
	}
	var suffix = capabilityName
	if suffix != "" {
		suffix = "/" + suffix
	}
	url := fmt.Sprintf("http://127.0.0.1%s/#/%s%s", Port, capabilityType, suffix)
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
		if err := OpenBrowser(url); err != nil {
			ioStreams.Infof("automatically invoking browser failed: %v\nPlease visit %s for reference docs", err, url)
		}
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGTERM)

	<-sc
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func launch(server *http.Server, errChan chan<- error) {
	go func() {
		err := server.ListenAndServe()
		if err != nil && errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()
}

func generateSideBar(capabilities []types.Capability, docsPath string) error {
	sideBar := filepath.Join(docsPath, SideBar)
	components, traits, workflowSteps, policies := getDefinitions(capabilities)
	f, err := os.Create(sideBar) // nolint
	if err != nil {
		return err
	}
	if _, err := f.WriteString("- Components Types\n"); err != nil {
		return err
	}

	for _, c := range components {
		if _, err := f.WriteString(fmt.Sprintf("  - [%s](%s/%s.md)\n", c, types.TypeComponentDefinition, c)); err != nil {
			return err
		}
	}
	if _, err := f.WriteString("- Traits\n"); err != nil {
		return err
	}
	for _, t := range traits {
		if _, err := f.WriteString(fmt.Sprintf("  - [%s](%s/%s.md)\n", t, types.TypeTrait, t)); err != nil {
			return err
		}
	}
	if _, err := f.WriteString("- Workflow Steps\n"); err != nil {
		return err
	}
	for _, t := range workflowSteps {
		if _, err := f.WriteString(fmt.Sprintf("  - [%s](%s/%s.md)\n", t, types.TypeWorkflowStep, t)); err != nil {
			return err
		}
	}

	if _, err := f.WriteString("- Policies\n"); err != nil {
		return err
	}
	for _, t := range policies {
		if _, err := f.WriteString(fmt.Sprintf("  - [%s](%s/%s.md)\n", t, types.TypePolicy, t)); err != nil {
			return err
		}
	}
	return nil
}

func generateNavBar(docsPath string) error {
	sideBar := filepath.Join(docsPath, NavBar)
	_, err := os.Create(sideBar) // nolint
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
  <link rel="stylesheet" href="./custom.css">
</head>
<body>
  <div id="app"></div>
  <script>
    window.$docsify = {
      name: 'KubeVela Customized Reference Docs',
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
  <!-- docgen -->
  <script src="//cdn.jsdelivr.net/npm/docsify-sidebar-collapse/dist/docsify-sidebar-collapse.min.js"></script>
</body>
</html>
`
	return os.WriteFile(filepath.Join(docsPath, IndexHTML), []byte(indexHTML), 0600)
}

func generateCustomCSS(docsPath string) error {
	css := `
body {
    overflow: auto !important;
}`
	return os.WriteFile(filepath.Join(docsPath, CSS), []byte(css), 0600)
}

func generateREADME(capabilities []types.Capability, docsPath string) error {
	readmeMD := filepath.Join(docsPath, README)
	f, err := os.Create(readmeMD) // nolint
	if err != nil {
		return err
	}
	if _, err := f.WriteString("# KubeVela Reference Docs for Component Types, Traits and WorkflowSteps\n" +
		"Click the navigation bar on the left or the links below to look into the detailed reference of a Workload type, Trait or Workflow Step.\n"); err != nil {
		return err
	}

	workloads, traits, workflowSteps, policies := getDefinitions(capabilities)

	if _, err := f.WriteString("## Component Types\n"); err != nil {
		return err
	}

	for _, w := range workloads {
		if _, err := f.WriteString(fmt.Sprintf("  - [%s](%s/%s.md)\n", w, types.TypeComponentDefinition, w)); err != nil {
			return err
		}
	}
	if _, err := f.WriteString("## Traits\n"); err != nil {
		return err
	}

	for _, t := range traits {
		if _, err := f.WriteString(fmt.Sprintf("  - [%s](%s/%s.md)\n", t, types.TypeTrait, t)); err != nil {
			return err
		}
	}

	if _, err := f.WriteString("## Workflow Steps\n"); err != nil {
		return err
	}
	for _, t := range workflowSteps {
		if _, err := f.WriteString(fmt.Sprintf("  - [%s](%s/%s.md)\n", t, types.TypeWorkflowStep, t)); err != nil {
			return err
		}
	}

	if _, err := f.WriteString("## Policies\n"); err != nil {
		return err
	}
	for _, t := range policies {
		if _, err := f.WriteString(fmt.Sprintf("  - [%s](%s/%s.md)\n", t, types.TypePolicy, t)); err != nil {
			return err
		}
	}

	return nil
}

func getDefinitions(capabilities []types.Capability) ([]string, []string, []string, []string) {
	var components, traits, workflowSteps, policies []string
	for _, c := range capabilities {
		switch c.Type {
		case types.TypeComponentDefinition:
			components = append(components, c.Name)
		case types.TypeTrait:
			traits = append(traits, c.Name)
		case types.TypeWorkflowStep:
			workflowSteps = append(workflowSteps, c.Name)
		case types.TypePolicy:
			policies = append(policies, c.Name)
		case types.TypeScope:
		case types.TypeWorkload:
		default:
		}
	}
	return components, traits, workflowSteps, policies
}

// ShowReferenceConsole will show capability reference in console
func ShowReferenceConsole(ctx context.Context, c common.Args, ioStreams cmdutil.IOStreams, capabilityName string, ns, location, i18nPath string, rev int64) error {
	cli, err := c.GetClient()
	if err != nil {
		return err
	}
	ref := &docgen.ConsoleReference{}
	paserRef, err := genRefParser(capabilityName, ns, location, i18nPath, rev)
	if err != nil {
		return err
	}
	paserRef.Client = cli
	ref.ParseReference = paserRef
	return ref.Show(ctx, c, ioStreams, capabilityName, ns, rev)
}

// ShowReferenceMarkdown will show capability in "markdown" format
func ShowReferenceMarkdown(ctx context.Context, c common.Args, ioStreams cmdutil.IOStreams, capabilityNameOrPath, outputPath, location, i18nPath, ns string, rev int64) error {
	ref := &docgen.MarkdownReference{}
	paserRef, err := genRefParser(capabilityNameOrPath, ns, location, i18nPath, rev)
	if err != nil {
		return err
	}
	ref.ParseReference = paserRef
	ref.DiscoveryMapper, err = c.GetDiscoveryMapper()
	if err != nil {
		return err
	}
	if err := ref.GenerateReferenceDocs(ctx, c, outputPath); err != nil {
		return errors.Wrap(err, "failed to generate reference docs")
	}
	if outputPath != "" {
		ioStreams.Infof("Generated docs in %s for %s in %s/%s.md\n", ref.I18N, capabilityNameOrPath, outputPath, ref.DefinitionName)
	}
	return nil
}

func genRefParser(capabilityNameOrPath, ns, location, i18nPath string, rev int64) (docgen.ParseReference, error) {
	ref := docgen.ParseReference{}
	if location != "" {
		docgen.LoadI18nData(i18nPath)
	}
	if strings.HasSuffix(capabilityNameOrPath, ".yaml") || strings.HasSuffix(capabilityNameOrPath, ".cue") {
		// read from local file
		localFilePath := capabilityNameOrPath
		fileName := filepath.Base(localFilePath)
		ref.DefinitionName = strings.TrimSuffix(strings.TrimSuffix(fileName, ".yaml"), ".cue")
		ref.Local = &docgen.FromLocal{Paths: []string{localFilePath}}
	} else {
		ref.DefinitionName = capabilityNameOrPath
		ref.Remote = &docgen.FromCluster{Namespace: ns, Rev: rev}
	}
	switch strings.ToLower(location) {
	case "zh", "cn", "chinese":
		ref.I18N = &docgen.Zh
	case "", "en", "english":
		ref.I18N = &docgen.En
	default:
		return ref, fmt.Errorf("unknown location %s for i18n translation", location)
	}
	return ref, nil
}

// OpenBrowser will open browser by url in different OS system
// nolint:gosec
func OpenBrowser(url string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("cmd", "/C", "start", url).Run()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	return err
}
