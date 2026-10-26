/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package application

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestGCWaitBackoffTime(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	testCases := map[string]struct {
		waiting time.Duration
		max     time.Duration
		want    time.Duration
	}{
		"just started waiting uses the base wait": {waiting: 0, max: time.Minute, want: 3 * time.Second},
		"short wait stays at the base wait":       {waiting: 10 * time.Second, max: time.Minute, want: 3 * time.Second},
		"wait grows with the time spent waiting":  {waiting: 2 * time.Minute, max: time.Minute, want: 30 * time.Second},
		"long wait is capped at the max":          {waiting: 17 * 24 * time.Hour, max: time.Minute, want: time.Minute},
		"max below the base wins":                 {waiting: time.Hour, max: time.Second, want: time.Second},
		"zero max disables the cap":               {waiting: time.Hour, max: 0, want: 15 * time.Minute},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			defer func(old time.Duration) { MaxGCWaitBackoffTime = old }(MaxGCWaitBackoffTime)
			MaxGCWaitBackoffTime = tc.max
			assert.Equal(t, tc.want, gcWaitBackoffTime(now.Add(-tc.waiting), now))
		})
	}
}

func TestGCWaitingSince(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	since := now.Add(-time.Hour)
	appWithReady := func(reason condition.ConditionReason, transition time.Time) *v1beta1.Application {
		app := &v1beta1.Application{}
		app.Status.SetConditions(condition.Condition{
			Type:               condition.TypeReady,
			Status:             corev1.ConditionFalse,
			Reason:             reason,
			LastTransitionTime: metav1.NewTime(transition),
		})
		return app
	}
	testCases := map[string]struct {
		app  *v1beta1.Application
		want time.Time
	}{
		"no stored application":                   {app: nil, want: now},
		"stored application has no conditions":    {app: &v1beta1.Application{}, want: now},
		"stored application is not waiting":       {app: appWithReady(condition.ReasonAvailable, since), want: now},
		"stored application is waiting":           {app: appWithReady(condition.ReasonDeleting, since), want: since},
		"waiting condition without a time is new": {app: appWithReady(condition.ReasonDeleting, time.Time{}), want: now},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.True(t, tc.want.Equal(gcWaitingSince(tc.app, now)))
		})
	}
}

func TestKeepConditionTransitionTimes(t *testing.T) {
	stored := metav1.NewTime(time.Date(2026, 10, 5, 20, 37, 39, 0, time.UTC))
	fresh := metav1.NewTime(time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC))
	deleting := condition.Condition{Type: condition.TypeReady, Status: corev1.ConditionFalse, Reason: condition.ReasonDeleting, Message: "waiting"}

	storedDeleting := deleting
	storedDeleting.LastTransitionTime = stored
	storedWithoutTime := deleting

	changedStatus := deleting
	changedStatus.Status = corev1.ConditionTrue
	changedReason := deleting
	changedReason.Reason = condition.ReasonAvailable

	testCases := map[string]struct {
		stored  []condition.Condition
		current condition.Condition
		want    metav1.Time
	}{
		"unchanged condition keeps the stored time":    {stored: []condition.Condition{storedDeleting}, current: deleting, want: stored},
		"changed message keeps the stored time":        {stored: []condition.Condition{storedDeleting}, current: deleting.WithMessage("still waiting"), want: stored},
		"changed status takes the new time":            {stored: []condition.Condition{storedDeleting}, current: changedStatus, want: fresh},
		"changed reason takes the new time":            {stored: []condition.Condition{storedDeleting}, current: changedReason, want: fresh},
		"condition missing from stored takes new time": {stored: nil, current: deleting, want: fresh},
		"stored condition without a time is ignored":   {stored: []condition.Condition{storedWithoutTime}, current: deleting, want: fresh},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			current := tc.current
			current.LastTransitionTime = fresh

			status := condition.ConditionedStatus{Conditions: []condition.Condition{current}}
			keepConditionTransitionTimes(&status, condition.ConditionedStatus{Conditions: tc.stored})
			assert.True(t, tc.want.Equal(&status.Conditions[0].LastTransitionTime))
		})
	}
}
