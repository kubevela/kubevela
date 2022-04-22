/*
Copyright 2021 The KubeVela Authors.

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

package collect

import (
	"context"
	"sort"
	"time"

	client2 "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/multicluster"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"

	"github.com/robfig/cron/v3"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

// TopKFrequent top frequency component or trait definition
var TopKFrequent = 5

// CrontabSpec the cron spec of job running
var CrontabSpec = "0 0 * * *"

// maximum tires is 5, initial duration is 1 minute
var waitBackOff = wait.Backoff{
	Steps:    5,
	Duration: 1 * time.Minute,
	Factor:   5.0,
	Jitter:   0.1,
}

// InfoCalculateCronJob is the cronJob to calculate the system info store in db
type InfoCalculateCronJob struct {
	ds datastore.DataStore
}

// StartCalculatingInfoCronJob will start the system info calculating job.
func StartCalculatingInfoCronJob(ds datastore.DataStore) {
	i := InfoCalculateCronJob{
		ds: ds,
	}

	// run calculate job in 0:00 of every day
	i.start(CrontabSpec)
}

func (i InfoCalculateCronJob) start(cronSpec string) {
	c := cron.New(cron.WithChain(
		// don't let job panic crash whole api-server process
		cron.Recover(cron.DefaultLogger),
	))

	// ignore the entityId and error, the cron spec is defined by hard code, mustn't generate error
	_, _ = c.AddFunc(cronSpec, func() {

		// ExponentialBackoff retry this job
		err := retry.OnError(waitBackOff, func(err error) bool {
			// always retry
			return true
		}, func() error {
			if err := i.run(); err != nil {
				log.Logger.Errorf("Fialed to calculate systemInfo, will try again after several minute error %v", err)
				return err
			}
			log.Logger.Info("Successfully to calculate systemInfo")
			return nil
		})
		if err != nil {
			log.Logger.Errorf("After 5 tries the calculating cronJob failed: %v", err)
		}
	})

	c.Start()
}

func (i InfoCalculateCronJob) run() error {
	ctx := context.Background()
	systemInfo := model.SystemInfo{}
	e, err := i.ds.List(ctx, &systemInfo, &datastore.ListOptions{})
	if err != nil {
		return err
	}
	// if no systemInfo means velaux have not have not send get request，so skip calculate job
	if len(e) == 0 {
		return nil
	}
	info, ok := e[0].(*model.SystemInfo)
	if !ok {
		return nil
	}
	// if disable collection skip calculate job
	if !info.EnableCollection {
		return nil
	}

	if err := i.calculateAndUpdate(ctx, *info); err != nil {
		return err
	}
	return nil
}

func (i InfoCalculateCronJob) calculateAndUpdate(ctx context.Context, systemInfo model.SystemInfo) error {

	appCount, topKComp, topKTrait, topWorkflowStep, topKPolicy, err := i.calculateAppInfo(ctx)
	if err != nil {
		return err
	}

	enabledAddon, err := i.calculateAddonInfo(ctx)
	if err != nil {
		return err
	}

	clusterCount, err := i.calculateClusterInfo(ctx)
	if err != nil {
		return err
	}

	statisticInfo := model.StatisticInfo{
		AppCount:            genCountInfo(appCount),
		TopKCompDef:         topKComp,
		TopKTraitDef:        topKTrait,
		TopKWorkflowStepDef: topWorkflowStep,
		TopKPolicyDef:       topKPolicy,
		ClusterCount:        genClusterCountInfo(clusterCount),
		EnabledAddon:        enabledAddon,
		UpdateTime:          time.Now(),
	}

	systemInfo.StatisticInfo = statisticInfo
	if err := i.ds.Put(ctx, &systemInfo); err != nil {
		return err
	}
	return nil
}

func (i InfoCalculateCronJob) calculateAppInfo(ctx context.Context) (int, []string, []string, []string, []string, error) {
	var err error
	var appCount int
	compDef := map[string]int{}
	traitDef := map[string]int{}
	workflowDef := map[string]int{}
	policyDef := map[string]int{}

	var app = model.Application{}
	entities, err := i.ds.List(ctx, &app, &datastore.ListOptions{})
	if err != nil {
		return 0, nil, nil, nil, nil, err
	}
	for _, entity := range entities {
		appModel, ok := entity.(*model.Application)
		if !ok {
			continue
		}
		appCount++
		comp := model.ApplicationComponent{
			AppPrimaryKey: appModel.Name,
		}
		comps, err := i.ds.List(ctx, &comp, &datastore.ListOptions{})
		if err != nil {
			return 0, nil, nil, nil, nil, err
		}
		for _, e := range comps {
			c, ok := e.(*model.ApplicationComponent)
			if !ok {
				continue
			}
			compDef[c.Type]++
			for _, t := range c.Traits {
				traitDef[t.Type]++
			}
		}

		workflow := model.Workflow{
			AppPrimaryKey: app.PrimaryKey(),
		}
		workflows, err := i.ds.List(ctx, &workflow, &datastore.ListOptions{})
		if err != nil {
			return 0, nil, nil, nil, nil, err
		}
		for _, e := range workflows {
			w, ok := e.(*model.Workflow)
			if !ok {
				continue
			}
			for _, step := range w.Steps {
				workflowDef[step.Type]++
			}
		}

		policy := model.ApplicationPolicy{
			AppPrimaryKey: app.PrimaryKey(),
		}
		policies, err := i.ds.List(ctx, &policy, &datastore.ListOptions{})
		if err != nil {
			return 0, nil, nil, nil, nil, err
		}
		for _, e := range policies {
			p, ok := e.(*model.ApplicationPolicy)
			if !ok {
				continue
			}
			policyDef[p.Type]++
		}
	}

	return appCount, topKFrequent(compDef, TopKFrequent), topKFrequent(traitDef, TopKFrequent), topKFrequent(workflowDef, TopKFrequent), topKFrequent(policyDef, TopKFrequent), nil
}

func (i InfoCalculateCronJob) calculateAddonInfo(ctx context.Context) (map[string]string, error) {
	client, err := clients.GetKubeClient()
	if err != nil {
		return nil, err
	}
	apps := &v1beta1.ApplicationList{}
	if err := client.List(ctx, apps, client2.InNamespace(types.DefaultKubeVelaNS), client2.HasLabels{oam.LabelAddonName}); err != nil {
		return nil, err
	}
	res := map[string]string{}
	for _, application := range apps.Items {
		if addonName := application.Labels[oam.LabelAddonName]; addonName != "" {
			var status string
			switch application.Status.Phase {
			case common.ApplicationRunning:
				status = "enabled"
			case common.ApplicationDeleting:
				status = "disabling"
			default:
				status = "enabling"
			}
			res[addonName] = status
		}
	}
	return res, nil
}

func (i InfoCalculateCronJob) calculateClusterInfo(ctx context.Context) (int, error) {
	client, err := clients.GetKubeClient()
	if err != nil {
		return 0, err
	}
	cs, err := multicluster.ListVirtualClusters(ctx, client)
	if err != nil {
		return 0, err
	}
	return len(cs), nil
}

type defPair struct {
	name  string
	count int
}

func topKFrequent(defs map[string]int, k int) []string {
	var pairs []defPair
	var res []string
	for name, num := range defs {
		pairs = append(pairs, defPair{name: name, count: num})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].count >= pairs[j].count
	})
	i := 0
	for _, pair := range pairs {
		res = append(res, pair.name)
		i++
		if i == k {
			break
		}
	}
	return res
}

func genCountInfo(num int) string {
	switch {
	case num < 10:
		return "<10"
	case num < 50:
		return "<50"
	case num < 100:
		return "<100"
	case num < 500:
		return "<500"
	case num < 2000:
		return "<2000"
	case num < 5000:
		return "<5000"
	case num < 10000:
		return "<10000"
	default:
		return ">=10000"
	}
}

func genClusterCountInfo(num int) string {
	switch {
	case num < 3:
		return "<3"
	case num < 10:
		return "<10"
	case num < 50:
		return "<50"
	default:
		return ">=50"
	}
}
