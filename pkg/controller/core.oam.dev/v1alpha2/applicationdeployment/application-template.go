package applicationdeployment

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// adjustTargetApplicationTemplate makes sure that the target template is in compliance before handing it over to
// the application controller
func (r *Reconciler) adjustTargetApplicationTemplate(ctx context.Context, targetWorkload *unstructured.Unstructured,
	targetApp *corev1alpha2.Application) error {
	klog.InfoS("Start to adjust the target application template",
		"application", klog.KObj(targetApp), "target workload", targetWorkload)

	// TODO: adjust the target workload if needed

	// update the template without the rollout annotation so that the application controller can take over
	anno := targetApp.GetAnnotations()
	delete(anno, oam.AnnotationAppRollout)
	targetApp.SetAnnotations(anno)
	return r.Update(ctx, targetApp)
}
