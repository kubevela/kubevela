package autoscalers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/go-logr/logr"
	kedav1alpha1 "github.com/kedacore/keda/api/v1alpha1"
	kedaclient "github.com/kedacore/keda/pkg/generated/clientset/versioned/typed/keda/v1alpha1"
	"github.com/oam-dev/kubevela/api/v1alpha1"
	"github.com/pkg/errors"
	autoscalingv2beta2 "k8s.io/api/autoscaling/v2beta2"
	apicorev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func (r *AutoscalerReconciler) scaleByKEDA(scaler v1alpha1.Autoscaler, namespace string, log logr.Logger) error {
	minReplicas := scaler.Spec.MinReplicas
	maxReplicas := scaler.Spec.MaxReplicas
	triggers := scaler.Spec.Triggers
	scalerName := scaler.Name
	targetWorkload := scaler.Spec.TargetWorkload

	var kedaTriggers []kedav1alpha1.ScaleTriggers
	var err error
	var reason string
	var resourceMetrics []*autoscalingv2beta2.ResourceMetricSource
	for _, t := range triggers {
		if t.Type == CronType {
			if kedaTriggers, err, reason = r.prepareKEDACronScalerTriggerSpec(scaler, t); err != nil {
				log.Error(err, reason)
				r.record.Event(&scaler, event.Warning(event.Reason(reason), err))
			}
		} else if t.Type == CPUType || t.Type == MemoryType || t.Type == StorageType || t.Type == EphemeralStorageType {
			resourceMetric := r.prepareKEDAResourceScalerMetrics(t)
			resourceMetrics = append(resourceMetrics, resourceMetric)
		}
	}

	scaleObj := kedav1alpha1.ScaledObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       scaledObjectKind,
			APIVersion: scaledObjectAPIVersion,
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
		Spec: kedav1alpha1.ScaledObjectSpec{
			ScaleTargetRef: &kedav1alpha1.ScaleTarget{
				APIVersion: targetWorkload.APIVersion,
				Kind:       targetWorkload.Kind,
				Name:       targetWorkload.Name,
			},
			MinReplicaCount: minReplicas,
			MaxReplicaCount: maxReplicas,
			Advanced: &kedav1alpha1.AdvancedConfig{
				HorizontalPodAutoscalerConfig: &kedav1alpha1.HorizontalPodAutoscalerConfig{
					ResourceMetrics: resourceMetrics,
				},
			},
			Triggers: kedaTriggers,
		},
	}

	config := r.config
	kedaClient, err := kedaclient.NewForConfig(config)
	if err != nil {
		log.Error(err, "failed to initiate a KEDA client", "config", config)
		return err
	}

	obj, err := kedaClient.ScaledObjects(namespace).Get(r.ctx, scalerName, metav1.GetOptions{})
	if err != nil {
		log.Info("KEDA ScaledObj doesn't exist", "ScaledObjectName", scalerName)
		if _, err := kedaClient.ScaledObjects(namespace).Create(r.ctx, &scaleObj, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			log.Error(err, "failed to create KEDA ScaledObj", "ScaledObject", scaleObj)
			return err
		}
	} else {
		obj.Spec = scaleObj.Spec
		if _, err := kedaClient.ScaledObjects(namespace).Update(r.ctx, obj, metav1.UpdateOptions{}); err != nil {
			log.Error(err, "failed to update KEDA ScaledObj", "ScaledObject", scaleObj)
			return err
		}
	}
	return nil
}

// prepareKEDACronScalerTriggerSpec converts Autoscaler spec into KEDA Cron scaler spec
func (r *AutoscalerReconciler) prepareKEDACronScalerTriggerSpec(scaler v1alpha1.Autoscaler, t v1alpha1.Trigger) ([]kedav1alpha1.ScaleTriggers, error, string) {
	var kedaTriggers []kedav1alpha1.ScaleTriggers
	targetWorkload := scaler.Spec.TargetWorkload
	if targetWorkload.Name == "" {
		err := errors.New(SpecWarningTargetWorkloadNotSet)
		return kedaTriggers, err, SpecWarningTargetWorkloadNotSet
	}

	triggerCondition := t.Condition.CronTypeCondition
	startAt := triggerCondition.StartAt
	if startAt == "" {
		return kedaTriggers, errors.New(SpecWarningStartAtTimeRequired), SpecWarningStartAtTimeRequired
	}
	duration := triggerCondition.Duration
	if duration == "" {
		return kedaTriggers, errors.New(SpecWarningDurationTimeRequired), SpecWarningDurationTimeRequired
	}
	var err error
	startTime, err := time.Parse("15:04", startAt)
	if err != nil {
		return kedaTriggers, err, SpecWarningStartAtTimeFormat
	}
	var startHour, startMinute int
	startHour = startTime.Hour()
	startMinute = startTime.Minute()

	durationTime, err := time.ParseDuration(duration)
	if err != nil {
		return kedaTriggers, err, SpecWarningDurationTimeNotInRightFormat
	}
	durationHour := durationTime.Hours()

	endHour := int(durationHour) + startHour
	if endHour >= 24 {
		return kedaTriggers, errors.New(SpecWarningSumOfStartAndDurationMoreThan24Hour), SpecWarningSumOfStartAndDurationMoreThan24Hour
	}
	replicas := triggerCondition.Replicas
	if replicas == 0 {
		return kedaTriggers, errors.New(SpecWarningReplicasRequired), SpecWarningReplicasRequired
	}

	timezone := triggerCondition.Timezone
	//if timezone == "" {
	//	timezone = "Asia/Shanghai"
	//}

	days := triggerCondition.Days
	var dayNo []int

	var i = 0
	// TODO(@zzxwill) On Mac, it's Sunday when i == 0, need check on Linux
	for _, d := range days {
		for i < 7 {
			if strings.EqualFold(time.Weekday(i).String(), d) {
				dayNo = append(dayNo, i)
				break
			}
			i++
		}
	}

	for _, n := range dayNo {
		kedaTrigger := kedav1alpha1.ScaleTriggers{
			Type: string(t.Type),
			Name: t.Name,
			Metadata: map[string]string{
				"timezone":        timezone,
				"start":           fmt.Sprintf("%d %d * * %d", startMinute, startHour, n),
				"end":             fmt.Sprintf("%d %d * * %d", startMinute, endHour, n),
				"desiredReplicas": strconv.Itoa(replicas),
			},
		}
		kedaTriggers = append(kedaTriggers, kedaTrigger)
	}
	return kedaTriggers, nil, ""
}

func (r *AutoscalerReconciler) prepareKEDAResourceScalerMetrics(t v1alpha1.Trigger) *autoscalingv2beta2.ResourceMetricSource {
	resourceMetric := &autoscalingv2beta2.ResourceMetricSource{
		Name: apicorev1.ResourceName(t.Type),
		Target: autoscalingv2beta2.MetricTarget{
			// Currently only CPU `Utilization` is supported
			Type:               CPUUtilization,
			AverageUtilization: t.Condition.Target,
		},
	}
	return resourceMetric
}
