package kube

import (
	"context"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/apply"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "kube"
)

type provider struct {
	deploy *apply.APIApplicator
	cli    client.Client
}

// Apply create or update CR in cluster.
func (h *provider) Apply(ctx wfContext.Context, v *value.Value, act types.Action) error {
	var workload = new(unstructured.Unstructured)
	if err := v.UnmarshalTo(workload); err != nil {
		return err
	}

	deployCtx := context.Background()
	if workload.GetNamespace() == "" {
		workload.SetNamespace("default")
	}
	if err := h.deploy.Apply(deployCtx, workload); err != nil {
		return err
	}
	return v.FillObject(workload.Object)
}

// Read get CR from cluster.
func (h *provider) Read(ctx wfContext.Context, v *value.Value, act types.Action) error {
	obj := new(unstructured.Unstructured)
	if err := v.UnmarshalTo(obj); err != nil {
		return err
	}
	key, err := client.ObjectKeyFromObject(obj)
	if err != nil {
		return err
	}
	if key.Namespace == "" {
		key.Namespace = "default"
	}
	if err := h.cli.Get(context.Background(), key, obj); err != nil {
		return err
	}
	return v.FillObject(obj.Object, "result")
}

// Install register handlers to provider discover.
func Install(p providers.Providers, cli client.Client) {
	prd := &provider{
		deploy: apply.NewAPIApplicator(cli),
		cli:    cli,
	}
	p.Register(ProviderName, map[string]providers.Handler{
		"apply": prd.Apply,
		"read":  prd.Read,
	})
}
