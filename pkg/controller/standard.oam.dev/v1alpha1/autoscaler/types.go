package autoscalers

import (
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// constants used in autoscaler controller
const (
	CronType v1alpha1.TriggerType = "cron"
	CPUType  v1alpha1.TriggerType = "cpu"
)
