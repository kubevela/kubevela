package cli

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	"github.com/fatih/color"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"

	"github.com/gosuri/uitable"
)

var _ = Describe("Output of listing addons tests", func() {
	// Output of function listAddons to test
	var actualTable *uitable.Table

	// getRowsByName extracts every rows with its NAME matching name
	getRowsByName := func(name string) []*uitable.Row {
		matchedRows := []*uitable.Row{}
		for _, row := range actualTable.Rows {
			// Check column NAME(0) = name
			if row.Cells[0].Data == name {
				matchedRows = append(matchedRows, row)
			}
		}
		return matchedRows
	}

	BeforeEach(func() {
		// Prepare KubeVela registry
		reg := &pkgaddon.Registry{
			Name: "KubeVela",
			Helm: &pkgaddon.HelmSource{
				URL: "https://addons.kubevela.net",
			},
		}
		ds := pkgaddon.NewRegistryDataStore(k8sClient)
		Expect(ds.AddRegistry(context.Background(), *reg)).To(Succeed())
	})

	JustBeforeEach(func() {
		// Print addon list to table for later comparison
		ret, err := listAddons(context.Background(), k8sClient, "")
		Expect(err).Should(BeNil())
		actualTable = ret
	})

	When("there is no addons installed", func() {
		It("should not have any enabled addon", func() {
			Expect(actualTable.Rows).ToNot(HaveLen(0))
			for idx, row := range actualTable.Rows {
				// Skip header
				if idx == 0 {
					continue
				}
				// Check column STATUS(4) = disabled
				Expect(row.Cells[4].Data).To(Equal("disabled"))
			}
		})
	})

	When("there is locally installed addons", func() {
		BeforeEach(func() {
			// Install fluxcd locally
			fluxcd := v1beta1.Application{}
			err := yaml.Unmarshal([]byte(fluxcdYaml), &fluxcd)
			Expect(err).Should(BeNil())
			Expect(k8sClient.Create(context.Background(), &fluxcd)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		})

		It("should print fluxcd addon as local", func() {
			matchedRows := getRowsByName("fluxcd")
			Expect(matchedRows).ToNot(HaveLen(0))
			// Only use first row (local first), check column REGISTRY(1) = local
			Expect(matchedRows[0].Cells[1].Data).To(Equal("local"))
			Eventually(func() error {
				matchedRows = getRowsByName("fluxcd")
				// Check column STATUS(4) = enabled
				if matchedRows[0].Cells[4].Data != "enabled" {
					return fmt.Errorf("fluxcd is not enabled yet")
				}
				// Check column AVAILABLE-VERSIONS(3) = 1.1.0
				if versionString := matchedRows[0].Cells[3].Data; versionString != fmt.Sprintf("[%s]", color.New(color.Bold, color.FgGreen).Sprintf("1.1.0")) {
					return fmt.Errorf("fluxcd version string is incorrect: %s", versionString)
				}
				return nil
			}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		})

		It("should print fluxcd in the registry as disabled", func() {
			matchedRows := getRowsByName("fluxcd")
			for idx, row := range matchedRows {
				// Skip local addon
				if idx == 0 {
					continue
				}
				Expect(row.Cells[1].Data).To(Equal("KubeVela"))
				Expect(row.Cells[4].Data).To(Equal("disabled"))
			}
		})
	})
})
