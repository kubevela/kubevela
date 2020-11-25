package autoscalers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	kedav1alpha1 "github.com/wonderflow/keda-api/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

func (r *AutoscalerReconciler) scaleByKEDA(scaler v1alpha1.Autoscaler, namespace string, log logr.Logger) error {
	ctx := context.Background()
	minReplicas := scaler.Spec.MinReplicas
	maxReplicas := scaler.Spec.MaxReplicas
	triggers := scaler.Spec.Triggers
	scalerName := scaler.Name
	targetWorkload := scaler.Spec.TargetWorkload

	var kedaTriggers []kedav1alpha1.ScaleTriggers
	var err error
	for _, t := range triggers {
		if t.Type == CronType {
			cronKedaTriggers, reason, err := r.prepareKEDACronScalerTriggerSpec(scaler, t)
			if err != nil {
				log.Error(err, reason)
				r.record.Event(&scaler, event.Warning(event.Reason(reason), err))
				return err
			}
			kedaTriggers = append(kedaTriggers, cronKedaTriggers...)
		} else {
			kedaTriggers = append(kedaTriggers, kedav1alpha1.ScaleTriggers{
				Type:     string(t.Type),
				Name:     t.Name,
				Metadata: t.Condition,

				//TODO(wonderflow): add auth in the future
				AuthenticationRef: nil,
			})
		}
	}
	spec := kedav1alpha1.ScaledObjectSpec{
		ScaleTargetRef: &kedav1alpha1.ScaleTarget{
			APIVersion: targetWorkload.APIVersion,
			Kind:       targetWorkload.Kind,
			Name:       targetWorkload.Name,
		},
		MinReplicaCount: minReplicas,
		MaxReplicaCount: maxReplicas,
		Triggers:        kedaTriggers,
	}
	var scaleObj kedav1alpha1.ScaledObject
	err = r.Client.Get(ctx, types.NamespacedName{Name: scalerName, Namespace: namespace}, &scaleObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			scaleObj := kedav1alpha1.ScaledObject{
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
				Spec: spec,
			}

			if err := r.Client.Create(ctx, &scaleObj); err != nil {
				log.Error(err, "failed to create KEDA ScaledObj", "ScaledObject", scaleObj)
				return err
			}
			log.Info("KEDA ScaledObj created", "ScaledObjectName", scalerName)
		}
	} else {
		scaleObj.Spec = spec
		if err := r.Client.Update(ctx, &scaleObj); err != nil {
			log.Error(err, "failed to update KEDA ScaledObj", "ScaledObject", scaleObj)
			return err
		}
		log.Info("KEDA ScaledObj updated", "ScaledObjectName", scalerName)
	}
	return nil
}

// CronTypeCondition defines the cron type for autoscaler
type CronTypeCondition struct {
	// StartAt is the time when the scaler starts, in format `"HHMM"` for example, "08:00"
	StartAt string `json:"startAt,omitempty"`

	// Duration means how long the target scaling will keep, after the time of duration, the scaling will stop
	Duration string `json:"duration,omitempty"`

	// Days means in which days the condition will take effect
	Days string `json:"days,omitempty"`

	// Replicas is the expected replicas
	Replicas string `json:"replicas,omitempty"`

	// Timezone defines the time zone, default to the timezone of the Kubernetes cluster
	Timezone string `json:"timezone,omitempty"`
}

// GetCronTypeCondition will get condition from map
func GetCronTypeCondition(condition map[string]string) (*CronTypeCondition, error) {
	data, err := json.Marshal(condition)
	if err != nil {
		return nil, err
	}
	var cronCon CronTypeCondition
	if err = json.Unmarshal(data, &cronCon); err != nil {
		return nil, err
	}
	return &cronCon, nil
}

// prepareKEDACronScalerTriggerSpec converts Autoscaler spec into KEDA Cron scaler spec
func (r *AutoscalerReconciler) prepareKEDACronScalerTriggerSpec(scaler v1alpha1.Autoscaler, t v1alpha1.Trigger) ([]kedav1alpha1.ScaleTriggers, string, error) {
	var kedaTriggers []kedav1alpha1.ScaleTriggers
	targetWorkload := scaler.Spec.TargetWorkload
	if targetWorkload.Name == "" {
		err := errors.New(SpecWarningTargetWorkloadNotSet)
		return kedaTriggers, SpecWarningTargetWorkloadNotSet, err
	}
	triggerCondition, err := GetCronTypeCondition(t.Condition)
	if err != nil {
		return nil, "convert cron condition failed", err
	}
	startAt := triggerCondition.StartAt
	if startAt == "" {
		return kedaTriggers, SpecWarningStartAtTimeRequired, errors.New(SpecWarningStartAtTimeRequired)
	}
	duration := triggerCondition.Duration
	if duration == "" {
		return kedaTriggers, SpecWarningDurationTimeRequired, errors.New(SpecWarningDurationTimeRequired)
	}
	startTime, err := time.Parse("15:04", startAt)
	if err != nil {
		return kedaTriggers, SpecWarningStartAtTimeFormat, err
	}
	var startHour, startMinute int
	startHour = startTime.Hour()
	startMinute = startTime.Minute()

	durationTime, err := time.ParseDuration(duration)
	if err != nil {
		return kedaTriggers, SpecWarningDurationTimeNotInRightFormat, err
	}
	durationHour := durationTime.Hours()
	durationMin := int(durationTime.Minutes()) % 60
	endMinite := startMinute + durationMin
	endHour := int(durationHour) + startHour

	if endMinite >= 60 {
		endMinite %= 60
		endHour++
	}
	var durationOneMoreDay int
	if endHour >= 24 {
		endHour %= 24
		durationOneMoreDay = 1
	}
	replicas, err := strconv.Atoi(triggerCondition.Replicas)
	if err != nil {
		return nil, "parse replica failed", err
	}
	if replicas == 0 {
		return kedaTriggers, SpecWarningReplicasRequired, errors.New(SpecWarningReplicasRequired)
	}

	timezone := triggerCondition.Timezone

	days := strings.Split(triggerCondition.Days, ",")
	var dayNo []int

	for i, d := range days {
		d = strings.TrimSpace(d)
		days[i] = d
		var found = false
		for i := 0; i < 7; i++ {
			if strings.EqualFold(time.Weekday(i).String(), d) {
				dayNo = append(dayNo, i)
				found = true
				break
			}
		}
		if !found {
			return nil, "", fmt.Errorf("wrong format %s, should be one of %v", d,
				[]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"})
		}
	}

	for idx, n := range dayNo {
		kedaTrigger := kedav1alpha1.ScaleTriggers{
			Type: string(t.Type),
			Name: t.Name + "-" + days[idx],
			Metadata: map[string]string{
				"timezone":        timezone,
				"start":           fmt.Sprintf("%d %d * * %d", startMinute, startHour, n),
				"end":             fmt.Sprintf("%d %d * * %d", endMinite, endHour, (n+durationOneMoreDay)%7),
				"desiredReplicas": strconv.Itoa(replicas),
			},
		}
		kedaTriggers = append(kedaTriggers, kedaTrigger)
	}
	return kedaTriggers, "", nil
}
