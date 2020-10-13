package autoscalers

import (
	"fmt"
	"strconv"
	"time"

	autoscalingv2beta2 "k8s.io/api/autoscaling/v2beta2"
	apicorev1 "k8s.io/api/core/v1"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/pointer"

	kedav1alpha1 "github.com/kedacore/keda/api/v1alpha1"

	"errors"
	"strings"

	"github.com/go-logr/logr"
	kedatype "github.com/kedacore/keda/pkg/generated/clientset/versioned/typed/keda/v1alpha1"
	"github.com/oam-dev/kubevela/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *AutoscalerReconciler) scaleByKEDA(scaler v1alpha1.Autoscaler, namespace string, log logr.Logger) error {
	config := r.config
	kedaClient, err := kedatype.NewForConfig(config)
	if err != nil {
		log.Error(err, "failed to initiate a KEDA client", "config", config)
		return err
	}

	minReplicas := scaler.Spec.MinReplicas
	maxReplicas := scaler.Spec.MaxReplicas
	triggers := scaler.Spec.Triggers
	scalerName := scaler.Name
	targetWorkload := scaler.Spec.TargetWorkload

	var resourceMetric *autoscalingv2beta2.ResourceMetricSource

	var kedaTriggers []kedav1alpha1.ScaleTriggers
	for _, t := range triggers {
		if t.Type == CronType {
			if targetWorkload.Name == "" {
				err := errors.New(SpecWarningTargetWorkloadNotSet)
				log.Error(err, "")
				r.record.Event(&scaler, event.Warning(SpecWarningTargetWorkloadNotSet, err))
				return err
			}

			triggerCondition := t.Condition.CronTypeCondition
			startAt := triggerCondition.StartAt
			if startAt == "" {
				return errors.New("spec.triggers.condition.startAt: Required value")
			}
			duration := triggerCondition.Duration
			if duration == "" {
				return errors.New("spec.triggers.condition.duration: Required value")
			}
			var err error
			startTime, err := time.Parse("15:04", startAt)
			if err != nil {
				log.Error(err, SpecWarningStartAtTimeFormat, startAt)
				r.record.Event(&scaler, event.Warning(SpecWarningStartAtTimeFormat, err))
				return err
			}
			var startHour, startMinute, durationHour int
			startHour = startTime.Hour()
			startMinute = startTime.Minute()
			if !strings.HasSuffix(duration, "h") {
				log.Error(err, "currently only hours of duration is supported.", "duration", duration)
				return err
			}

			splitDuration := strings.Split(duration, "h")
			if len(splitDuration) != 2 {
				log.Error(err, "duration hour is not in the right format, like `12h`.", "duration", duration)
				return err
			}
			if durationHour, err = strconv.Atoi(splitDuration[0]); err != nil {
				log.Error(err, "duration hour is not in the right format, like `12h`.", "duration", duration)
				return err
			}

			endHour := durationHour + startHour
			if endHour >= 24 {
				log.Error(err, "the sum of the hour of startAt and duration hour has to be less than 24 hours.", "startAt", startAt, "duration", duration)
				return err
			}
			replicas := triggerCondition.Replicas
			if replicas == 0 {
				return errors.New("spec.triggers.condition.replicas: Required value")
			}

			timezone := triggerCondition.Timezone
			if timezone == "" {
				return errors.New("spec.triggers.condition.timezone: Required value")
			}

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
		} else if t.Type == CPUType || t.Type == MemoryType || t.Type == StorageType || t.Type == EphemeralStorageType {
			resourceMetric = &autoscalingv2beta2.ResourceMetricSource{
				Name: apicorev1.ResourceName(string(t.Type)),
				Target: autoscalingv2beta2.MetricTarget{
					// Currently only CPU `Utilization` is supported
					Type:               CPUUtilization,
					AverageUtilization: t.Condition.Target,
				},
			}
		}
	}

	scaleTarget := kedav1alpha1.ScaleTarget{
		APIVersion: targetWorkload.APIVersion,
		Kind:       targetWorkload.Kind,
		Name:       targetWorkload.Name,
	}

	var resourceMetrics []*autoscalingv2beta2.ResourceMetricSource
	resourceMetrics = append(resourceMetrics, resourceMetric)
	advanced := &kedav1alpha1.AdvancedConfig{
		HorizontalPodAutoscalerConfig: &kedav1alpha1.HorizontalPodAutoscalerConfig{
			ResourceMetrics: resourceMetrics,
		},
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
			ScaleTargetRef:  &scaleTarget,
			MinReplicaCount: minReplicas,
			MaxReplicaCount: maxReplicas,
			Advanced:        advanced,
			Triggers:        kedaTriggers,
		},
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
