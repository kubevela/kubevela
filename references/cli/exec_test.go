package cli

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubectl/pkg/cmd/exec"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestExecCommand(t *testing.T) {
	tf := cmdtesting.NewTestFactory()
	defer tf.Cleanup()
	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	fakeC := types.Args{
		Config: tf.ClientConfigVal,
	}
	cmd := NewExecCommand(fakeC, io)
	cmd.PersistentFlags().StringP("env", "e", "", "")
	o := &VelaExecOptions{
		kcExecOptions: &exec.ExecOptions{},
		f:             tf,
		ClientSet: fake.NewSimpleClientset(&corev1.PodList{
			Items: []corev1.Pod{
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "fakePod",
						Namespace: "default",
						Labels: map[string]string{
							oam.LabelAppName:      "fakeApp",
							oam.LabelAppComponent: "fakeComp",
						}},
				},
			},
		}),
	}
	err := o.Init(context.Background(), cmd, []string{"fakeApp"})
	errString := fmt.Sprintf(`application "%s" not found`, "fakeApp")
	assert.EqualError(t, err, errString)
}

func TestExecCommandPersistentPreRunE(t *testing.T) {
	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	fakeC := types.Args{}
	cmd := NewExecCommand(fakeC, io)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}
