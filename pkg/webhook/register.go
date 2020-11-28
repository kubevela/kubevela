package webhook

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/oam-dev/kubevela/pkg/webhook/standard.oam.dev/v1alpha1/metrics"
	"github.com/oam-dev/kubevela/pkg/webhook/standard.oam.dev/v1alpha1/podspecworkload"
)

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-standard-oam-dev-v1alpha1-metricstrait,mutating=false,failurePolicy=fail,groups=standard.oam.dev,resources=metricstraits,versions=v1alpha1,name=vmetricstrait.kb.io
// +kubebuilder:webhook:path=/mutate-standard-oam-dev-v1alpha1-metricstrait,mutating=true,failurePolicy=fail,groups=standard.oam.dev,resources=metricstraits,verbs=create;update,versions=v1alpha1,name=mmetricstrait.kb.io
// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-standard-oam-dev-v1alpha1-podspecworkload,mutating=false,failurePolicy=fail,groups=standard.oam.dev,resources=PodSpecWorkload,versions=v1alpha1,name=vpodspecworkload.kb.io
// +kubebuilder:webhook:path=/mutate-standard-oam-dev-v1alpha1-podspecworkload,mutating=true,failurePolicy=fail,groups=standard.oam.dev,resources=PodSpecWorkload,verbs=create;update,versions=v1alpha1,name=mpodspecworkload.kb.io

// Register will register all the services to the webhook server
func Register(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	// MetricsTrait
	server.Register("/validate-standard-oam-dev-v1alpha1-metricstrait",
		&webhook.Admission{Handler: &metrics.ValidatingHandler{}})
	server.Register("/mutate-standard-oam-dev-v1alpha1-metricstrait",
		&webhook.Admission{Handler: &metrics.MutatingHandler{}})
	// PodSpecWorkload
	server.Register("/validate-standard-oam-dev-v1alpha1-podspecworkload",
		&webhook.Admission{Handler: &podspecworkload.ValidatingHandler{}})
	server.Register("/mutate-standard-oam-dev-v1alpha1-podspecworkload",
		&webhook.Admission{Handler: &podspecworkload.MutatingHandler{}})
}
