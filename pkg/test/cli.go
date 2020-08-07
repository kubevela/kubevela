package test

import (
	"context"
	"strings"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/spf13/cobra"
)

// CliTest test engine
type CliTest interface {
	Run()
}

// InitResources resource to init before run testing
type InitResources struct {
	Create []runtime.Object
	Update []runtime.Object
}

// CliTestCase is testing case for cli
type CliTestCase struct {
	// Resources to init
	Resources InitResources
	// ExpectedExistResources expected exist resources
	ExpectedExistResources []runtime.Object
	// ExpectedResources expected resources exist and equal
	ExpectedResources []runtime.Object
	// WantException expected exit with error
	WantException bool
	// ExpectedOutput output equal to
	ExpectedOutput string
	// ExpectedString output contains strings
	ExpectedString string
	Args           []string
	// Namespaces to run
	Namespaces string
}

type clitestImpl struct {
	cases   map[string]*CliTestCase
	t       *testing.T
	scheme  *runtime.Scheme
	command func(c client.Client, ioStreams cmdutil.IOStreams, args []string) *cobra.Command
}

// NewCliTest return cli testimpl
func NewCliTest(t *testing.T, scheme *runtime.Scheme,
	command func(c client.Client, ioStreams cmdutil.IOStreams,
		args []string) *cobra.Command, cases map[string]*CliTestCase) CliTest {
	return &clitestImpl{
		cases:   cases,
		t:       t,
		scheme:  scheme,
		command: command,
	}
}

// Run testing
func (c *clitestImpl) Run() {
	for name, tc := range c.cases {
		c.t.Run(name, func(t *testing.T) {
			fakeClient := fake.NewFakeClientWithScheme(c.scheme)
			iostream, _, outPut, _ := cmdutil.NewTestIOStreams()

			// init resources
			if len(tc.Resources.Create) != 0 {
				for _, resource := range tc.Resources.Create {
					err := fakeClient.Create(context.TODO(), resource)
					assert.NilError(t, err)
				}
			}
			if len(tc.Resources.Update) != 0 {
				for _, resource := range tc.Resources.Update {
					err := fakeClient.Update(context.TODO(), resource)
					assert.NilError(t, err)
				}
			}

			// init command
			runCmd := c.command(fakeClient, iostream, tc.Args)
			runCmd.SetOutput(outPut)
			err := runCmd.Execute()

			// check expected resources
			if len(tc.ExpectedExistResources) != 0 {
				for _, expectedResource := range tc.ExpectedExistResources {
					object, _ := expectedResource.(metav1.Object)

					resource := expectedResource.DeepCopyObject()
					err := fakeClient.Get(context.TODO(), client.ObjectKey{Namespace: object.GetNamespace(), Name: object.GetName()}, resource)
					assert.NilError(t, err)
				}
			}

			// check exit output
			errTip := tc.ExpectedString
			if tc.ExpectedOutput != "" {
				errTip = tc.ExpectedOutput
			}
			if tc.WantException {
				assert.ErrorContains(t, err, errTip)
				return
			}

			// check output messages
			if tc.ExpectedOutput != "" {
				assert.Equal(t, tc.ExpectedOutput, outPut.String(), name)
				return
			}

			assert.Equal(t, true, strings.Contains(outPut.String(), tc.ExpectedString))
		})
	}
}
