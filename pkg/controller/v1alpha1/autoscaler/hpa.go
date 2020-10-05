package controllers

import (
	"reflect"

	"github.com/go-logr/logr"
	"github.com/oam-dev/kubevela/api/v1alpha1"
	v1 "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
)

func (r *AutoscalerReconciler) scaleByHPA(scaler v1alpha1.Autoscaler, namespace string, log logr.Logger) error {
	config := r.config
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error(err, "failed to initiate a clientSet", "config", config)
		return err
	}

	minReplicas := scaler.Spec.MinReplicas
	maxReplicas := *scaler.Spec.MaxReplicas
	triggers := scaler.Spec.Triggers
	scalerName := scaler.Name
	targetWorkload := scaler.Spec.TargetWorkload

	for _, t := range triggers {
		if t.Type == CPUType || t.Type == MemoryType || t.Type == StorageType || t.Type == EphemeralStorageType {
			triggerCondition := t.Condition.DefaultCondition
			target := triggerCondition.Target

			scaleTarget := v1.CrossVersionObjectReference{
				Name:       targetWorkload.Name,
				APIVersion: targetWorkload.APIVersion,
				Kind:       targetWorkload.Kind,
			}

			scalerObj := v1.HorizontalPodAutoscaler{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       reflect.TypeOf(v1.HorizontalPodAutoscaler{}).Name(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      scalerName,
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         scaler.APIVersion,
							Kind:               scaler.Kind,
							UID:                scaler.GetUID(),
							Name:               scalerName,
							Controller:         pointer.BoolPtr(true),
							BlockOwnerDeletion: pointer.BoolPtr(true),
						},
					},
				},
				Spec: v1.HorizontalPodAutoscalerSpec{
					ScaleTargetRef:                 scaleTarget,
					MinReplicas:                    minReplicas,
					MaxReplicas:                    maxReplicas,
					TargetCPUUtilizationPercentage: target,
				},
			}
			if obj, err := clientSet.AutoscalingV1().HorizontalPodAutoscalers(namespace).
				Create(r.ctx, &scalerObj, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
				log.Error(err, "failed to create HPA instance", "HPA", obj)
				return err
			}

			if obj, err := clientSet.AutoscalingV1().HorizontalPodAutoscalers(namespace).
				Update(r.ctx, &scalerObj, metav1.UpdateOptions{}); err != nil {
				log.Error(err, "failed to update HPA instance", "HPA", obj)
				return err
			}
		}
	}
	return nil
}
