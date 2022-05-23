/*
Copyright 2022 The KubeVela Authors.

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

package sync

import (
	"context"
	"time"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
)

// WorkflowRecordSync sync workflow record from cluster to database
type WorkflowRecordSync struct {
	Duration        time.Duration
	WorkflowService service.WorkflowService `inject:""`
}

// Start sync workflow record data
func (w *WorkflowRecordSync) Start(ctx context.Context, errorChan chan error) {
	log.Logger.Infof("workflow record syncing worker started")
	defer log.Logger.Infof("workflow record syncing worker closed")
	t := time.NewTicker(w.Duration)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if err := w.WorkflowService.SyncWorkflowRecord(ctx); err != nil {
				log.Logger.Errorf("syncWorkflowRecordError: %s", err.Error())
			}
		case <-ctx.Done():
			return
		}
	}
}
