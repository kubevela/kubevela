package commands

import (
	"fmt"
	"os"
	"testing"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/commands/util"
)

func TestUp(t *testing.T) {
	client := fake.NewFakeClientWithScheme(scheme.Scheme)
	ioStream := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	env := types.EnvMeta{
		Name:      "up",
		Namespace: "env-up",
		Issuer:    "up",
	}
	o := AppfileOptions{
		Kubecli: client,
		IO:      ioStream,
		Env:     &env,
	}
	appName := "app-up"
	services := []*v1alpha2.Component{
		{
			TypeMeta: v1.TypeMeta{Kind: "Kind1", APIVersion: "v1"},
		},
	}
	msg := o.Info(appName, services)
	assert.Contains(t, msg, "App has been deployed")
	assert.Contains(t, msg, fmt.Sprintf("App status: vela status %s", appName))
}
