package cli

import (
	"context"
	"testing"

	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
)

func TestLlistProviders(t *testing.T) {
	ctx := context.Background()
	type args struct {
		k8sClient client.Client
	}
	type want struct {
		errMsg string
	}
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)
	corev1.AddToScheme(s)
	terraformapi.AddToScheme(s)
	p1 := &terraformapi.Provider{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Provider",
			APIVersion: "terraform.core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "default",
			Labels: map[string]string{
				"config.oam.dev/type": types.TerraformProvider,
			},
		},
	}
	p2 := &terraformapi.Provider{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Provider",
			APIVersion: "terraform.core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p2",
			Namespace: "default",
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(p1, p2).Build()

	testcases := map[string]struct {
		args args
		want want
	}{
		"success": {
			args: args{
				k8sClient: k8sClient,
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			err := listProviders(ctx, tc.args.k8sClient)
			if err != nil || tc.want.errMsg != "" {
				assert.Contains(t, err.Error(), tc.want.errMsg)
			}
		})
	}
}
