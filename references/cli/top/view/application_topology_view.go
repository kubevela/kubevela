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

package view

import (
	"context"
	"fmt"
	"time"

	"github.com/bluele/gcache"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/model"
	clicommon "github.com/oam-dev/kubevela/references/common"
)

// TopologyView display the resource topology of application
type TopologyView struct {
	*tview.Grid
	app                      *App
	actions                  model.KeyActions
	ctx                      context.Context
	focusTopology            bool
	formatter                *component.TopologyTreeNodeFormatter
	cache                    gcache.Cache // lru cache with expired time
	metricsInstance          *tview.Table
	appTopologyInstance      *TopologyTree
	resourceTopologyInstance *TopologyTree
	cancelFunc               func() // auto refresh cancel function
}

type cacheView struct {
	appTopologyInstance      *TopologyTree
	resourceTopologyInstance *TopologyTree
}

// TopologyTree is the abstract of topology tree
type TopologyTree struct {
	*tview.TreeView
}

const (
	numberOfCacheView = 10
	// cache expire time
	expireTime = 10
	// request timeout time
	topologyReqTimeout = 5
)

var (
	topologyViewInstance = new(TopologyView)
)

// NewTopologyView return a new topology view
func NewTopologyView(ctx context.Context, app *App) model.View {
	topologyViewInstance.app = app
	topologyViewInstance.ctx = ctx

	if topologyViewInstance.Grid == nil {
		topologyViewInstance.Grid = tview.NewGrid()
		topologyViewInstance.actions = make(model.KeyActions)
		topologyViewInstance.formatter = component.NewTopologyTreeNodeFormatter(app.config.Theme)
		topologyViewInstance.cache = gcache.New(numberOfCacheView).LRU().Expiration(expireTime * time.Second).Build()
		topologyViewInstance.metricsInstance = tview.NewTable()
		topologyViewInstance.appTopologyInstance = new(TopologyTree)
		topologyViewInstance.resourceTopologyInstance = new(TopologyTree)

		topologyViewInstance.Init()
	}
	return topologyViewInstance
}

// Init the topology view
func (v *TopologyView) Init() {
	v.metricsInstance.SetFixed(2, 6)
	v.metricsInstance.SetBorder(true).SetBorderColor(v.app.config.Theme.Border.Table.Color())

	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetRows(0).SetColumns(-1, -1)
	v.SetBorder(true)
	v.SetBorderAttributes(tcell.AttrItalic)
	v.SetBorderColor(v.app.config.Theme.Border.Table.Color())
	v.SetTitle(title)
	v.SetTitleColor(v.app.config.Theme.Table.Title.Color())
	v.bindKeys()
	v.SetInputCapture(v.keyboard)
}

// Start the topology view
func (v *TopologyView) Start() {
	v.Update(func() {})
	v.AutoRefresh(v.Update)
}

// Stop the topology view
func (v *TopologyView) Stop() {
	v.Grid.Clear()
	v.cancelFunc()
}

// Hint return the menu hints of topology view
func (v *TopologyView) Hint() []model.MenuHint {
	return v.actions.Hint()
}

// Name return the name of topology view
func (v *TopologyView) Name() string {
	return "Topology"
}

// Update the topology view
func (v *TopologyView) Update(timeoutCancel func()) {
	appName := v.ctx.Value(&model.CtxKeyAppName).(string)
	namespace := v.ctx.Value(&model.CtxKeyNamespace).(string)
	key := fmt.Sprintf("%s-%s", appName, namespace)

	value, err := v.cache.Get(key)

	generateTopology := func() {
		v.resourceTopologyInstance = v.NewResourceTopologyView()
		v.appTopologyInstance = v.NewAppTopologyView()
		// add new topology view to cache
		_ = v.cache.Set(key, &cacheView{
			resourceTopologyInstance: v.resourceTopologyInstance,
			appTopologyInstance:      v.appTopologyInstance,
		})
	}

	if err != nil {
		generateTopology()
	} else {
		view, ok := value.(*cacheView)
		if ok {
			v.appTopologyInstance = view.appTopologyInstance
			v.resourceTopologyInstance = view.resourceTopologyInstance
		} else {
			generateTopology()
		}
	}
	v.updateMetrics(appName, namespace)

	v.Grid.AddItem(v.metricsInstance, 0, 0, 1, 2, 0, 0, false)
	v.Grid.AddItem(v.appTopologyInstance, 1, 0, 7, 1, 0, 0, true)
	v.Grid.AddItem(v.resourceTopologyInstance, 1, 1, 7, 1, 0, 0, true)

	// reset focus
	if v.focusTopology {
		v.app.SetFocus(v.resourceTopologyInstance)
	} else {
		v.app.SetFocus(v.appTopologyInstance)
	}
	// ctx done
	timeoutCancel()
}

func (v *TopologyView) keyboard(event *tcell.EventKey) *tcell.EventKey {
	key := event.Key()
	if key == tcell.KeyUp || key == tcell.KeyDown {
		return event
	}
	if a, ok := v.actions[component.StandardizeKey(event)]; ok {
		return a.Action(event)
	}
	return event
}

func (v *TopologyView) bindKeys() {
	v.actions.Delete([]tcell.Key{tcell.KeyEnter})
	v.actions.Add(model.KeyActions{
		component.KeyQ:    model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
		tcell.KeyTAB:      model.KeyAction{Description: "Switch", Action: v.switchTopology, Visible: true, Shared: true},
	})
}

func (v *TopologyView) updateMetrics(appName, namespace string) {
	app := new(v1beta1.Application)
	err := v.app.client.Get(context.Background(), client.ObjectKey{
		Name:      appName,
		Namespace: namespace,
	}, app)
	if err != nil {
		return
	}

	metrics, err := clicommon.GetApplicationMetrics(v.app.client, v.app.config.RestConfig, app)
	if err != nil {
		return
	}

	format := "%10s : %10d"
	cell := tview.NewTableCell(fmt.Sprintf(format, "Node",
		metrics.ResourceNum.Node)).SetAlign(tview.AlignLeft).SetExpansion(3)
	v.metricsInstance.SetCell(0, 0, cell)
	cell = tview.NewTableCell(fmt.Sprintf(format, "Cluster", metrics.ResourceNum.Cluster)).SetAlign(tview.AlignLeft).SetExpansion(3)
	v.metricsInstance.SetCell(1, 0, cell)

	cell = tview.NewTableCell(fmt.Sprintf(format, "Pod", metrics.ResourceNum.Pod)).SetAlign(tview.AlignLeft).SetExpansion(3)
	v.metricsInstance.SetCell(0, 1, cell)
	cell = tview.NewTableCell(fmt.Sprintf(format, "Container", metrics.ResourceNum.Container)).SetAlign(tview.AlignLeft).SetExpansion(3)
	v.metricsInstance.SetCell(1, 1, cell)

	format = "%20s : %10s"
	cell = tview.NewTableCell(fmt.Sprintf(format, "Managed Resource", fmt.Sprintf("%d", metrics.ResourceNum.Subresource))).SetAlign(tview.AlignLeft).SetExpansion(3)
	v.metricsInstance.SetCell(0, 2, cell)
	cell = tview.NewTableCell(fmt.Sprintf(format, "Storage", fmt.Sprintf("%dGi", metrics.Metrics.Storage/(1024*1024*1024)))).SetAlign(tview.AlignLeft).SetExpansion(3)
	v.metricsInstance.SetCell(1, 2, cell)

	format = "%10s : %10s"
	cell = tview.NewTableCell(fmt.Sprintf(format, "CPU", fmt.Sprintf("%dm", metrics.Metrics.CPUUsage))).SetAlign(tview.AlignLeft).SetExpansion(3)
	v.metricsInstance.SetCell(0, 3, cell)
	cell = tview.NewTableCell(fmt.Sprintf(format, "Memory", fmt.Sprintf("%dMi", metrics.Metrics.MemoryUsage/(1024*1024)))).SetAlign(tview.AlignLeft).SetExpansion(3)
	v.metricsInstance.SetCell(1, 3, cell)

	format = "%20s : %10s"
	cell = tview.NewTableCell(fmt.Sprintf(format, "CPU Limit", fmt.Sprintf("%dm", metrics.Metrics.CPULimit))).SetAlign(tview.AlignLeft).SetExpansion(3)
	v.metricsInstance.SetCell(0, 4, cell)
	cell = tview.NewTableCell(fmt.Sprintf(format, "Memory Limit", fmt.Sprintf("%dMi", metrics.Metrics.MemoryLimit/(1024*1024)))).SetAlign(tview.AlignLeft).SetExpansion(3)
	v.metricsInstance.SetCell(1, 4, cell)

	cell = tview.NewTableCell(fmt.Sprintf(format, "CPU Request", fmt.Sprintf("%dm", metrics.Metrics.CPURequest))).SetAlign(tview.AlignLeft).SetExpansion(3)
	v.metricsInstance.SetCell(0, 5, cell)
	cell = tview.NewTableCell(fmt.Sprintf(format, "Memory Request", fmt.Sprintf("%dMi", metrics.Metrics.MemoryRequest/(1024*1024)))).SetAlign(tview.AlignLeft).SetExpansion(3)
	v.metricsInstance.SetCell(1, 5, cell)
}

// NewResourceTopologyView return a new resource topology view
func (v *TopologyView) NewResourceTopologyView() *TopologyTree {
	newTopology := new(TopologyTree)
	appName := v.ctx.Value(&model.CtxKeyAppName).(string)
	namespace := v.ctx.Value(&model.CtxKeyNamespace).(string)

	newTopology.TreeView = tview.NewTreeView()
	newTopology.SetGraphics(true)
	newTopology.SetGraphicsColor(v.app.config.Theme.Topology.Line.Color())
	newTopology.SetBorder(true)
	newTopology.SetBorderColor(v.app.config.Theme.Border.Table.Color())
	newTopology.SetTitle(fmt.Sprintf("[ %s ]", "Resource"))
	newTopology.SetTitleColor(v.app.config.Theme.Table.Title.Color())

	root := tview.NewTreeNode(v.formatter.EmojiFormat(fmt.Sprintf("%s (%s)", appName, namespace), "app")).SetSelectable(true)
	newTopology.SetRoot(root)

	resourceTree, err := model.ApplicationResourceTopology(v.app.client, appName, namespace)
	if err == nil {
		for _, resource := range resourceTree {
			root.AddChild(v.buildTopology(resource.ResourceTree))
		}
	}
	return newTopology
}

// NewAppTopologyView return a new app topology view
func (v *TopologyView) NewAppTopologyView() *TopologyTree {
	newTopology := new(TopologyTree)
	appName := v.ctx.Value(&model.CtxKeyAppName).(string)
	namespace := v.ctx.Value(&model.CtxKeyNamespace).(string)

	newTopology.TreeView = tview.NewTreeView()
	newTopology.SetGraphics(true)
	newTopology.SetGraphicsColor(v.app.config.Theme.Topology.Line.Color())
	newTopology.SetBorder(true)
	newTopology.SetBorderColor(v.app.config.Theme.Border.Table.Color())
	newTopology.SetTitle(fmt.Sprintf("[ %s ]", "App"))
	newTopology.SetTitleColor(v.app.config.Theme.Table.Title.Color())

	root := tview.NewTreeNode(v.formatter.EmojiFormat(fmt.Sprintf("%s (%s)", appName, namespace), "app")).SetSelectable(true)

	newTopology.SetRoot(root)

	app, err := model.LoadApplication(v.app.client, appName, namespace)
	if err != nil {
		return newTopology
	}
	// workflow
	workflowNode := tview.NewTreeNode(v.formatter.EmojiFormat("WorkFlow", "workflow")).SetSelectable(true)
	root.AddChild(workflowNode)
	for _, step := range app.Status.Workflow.Steps {
		stepNode := tview.NewTreeNode(component.WorkflowStepFormat(step.Name, step.Phase))
		for _, subStep := range step.SubStepsStatus {
			subStepNode := tview.NewTreeNode(subStep.Name)
			stepNode.AddChild(subStepNode)
		}
		workflowNode.AddChild(stepNode)
	}

	// component
	componentTitleNode := tview.NewTreeNode(v.formatter.EmojiFormat("Component", "component")).SetSelectable(true)
	root.AddChild(componentTitleNode)
	for _, c := range app.Spec.Components {
		cNode := tview.NewTreeNode(c.Name)
		attrNode := tview.NewTreeNode("Attributes")
		attrNode.AddChild(tview.NewTreeNode(fmt.Sprintf("Type: %s", c.Type)))
		cNode.AddChild(attrNode)

		if len(c.Traits) > 0 {
			traitTitleNode := tview.NewTreeNode(v.formatter.EmojiFormat("Trait", "trait")).SetSelectable(true)
			cNode.AddChild(traitTitleNode)
			for _, trait := range c.Traits {
				traitNode := tview.NewTreeNode(trait.Type)
				traitTitleNode.AddChild(traitNode)
			}
		}

		componentTitleNode.AddChild(cNode)
	}

	// policy
	policyNode := tview.NewTreeNode(v.formatter.EmojiFormat("Policy", "policy")).SetSelectable(true)
	root.AddChild(policyNode)
	for _, policy := range app.Spec.Policies {
		policyNode.AddChild(tview.NewTreeNode(policy.Name))
	}
	return newTopology
}

func (v *TopologyView) switchTopology(_ *tcell.EventKey) *tcell.EventKey {
	if v.focusTopology {
		v.app.SetFocus(v.appTopologyInstance)
	} else {
		v.app.SetFocus(v.resourceTopologyInstance)
	}
	v.focusTopology = !v.focusTopology
	return nil
}

func (v *TopologyView) buildTopology(node *types.ResourceTreeNode) *tview.TreeNode {
	if node == nil {
		return tview.NewTreeNode("?")
	}
	rootNode := tview.NewTreeNode(v.formatter.EmojiFormat(node.Name, node.Kind)).SetSelectable(true)

	attrNode := tview.NewTreeNode("Attributes")
	attrNode.AddChild(tview.NewTreeNode(fmt.Sprintf("Kind: %s", v.formatter.ColorizeKind(node.Kind))))
	attrNode.AddChild(tview.NewTreeNode(fmt.Sprintf("API Version: %s", node.APIVersion)))
	attrNode.AddChild(tview.NewTreeNode(fmt.Sprintf("Namespace: %s", node.Namespace)))
	attrNode.AddChild(tview.NewTreeNode(fmt.Sprintf("Cluster: %s", node.Cluster)))
	attrNode.AddChild(tview.NewTreeNode(fmt.Sprintf("Status: %s", v.formatter.ColorizeStatus(node.HealthStatus.Status))))

	rootNode.AddChild(attrNode)
	if len(node.LeafNodes) > 0 {
		subNode := tview.NewTreeNode("Sub Resource")
		rootNode.AddChild(subNode)

		for _, sub := range node.LeafNodes {
			subNode.AddChild(v.buildTopology(sub))
		}
	}

	return rootNode
}

// Refresh the topology  view
func (v *TopologyView) Refresh(clear bool, update func(timeoutCancel func())) {
	if clear {
		v.Grid.Clear()
	}

	updateWithTimeout := func() {
		ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*topologyReqTimeout)
		defer cancelFunc()
		go update(cancelFunc)

		select {
		case <-time.After(time.Second * topologyReqTimeout): // timeout
		case <-ctx.Done(): // success
		}
	}

	v.app.QueueUpdateDraw(updateWithTimeout)
}

// AutoRefresh will refresh the view in every RefreshDelay delay
func (v *TopologyView) AutoRefresh(update func(timeoutCancel func())) {
	var ctx context.Context
	ctx, v.cancelFunc = context.WithCancel(context.Background())
	go func() {
		for {
			time.Sleep(RefreshDelay * time.Second)
			select {
			case <-ctx.Done():
				return
			default:
				v.Refresh(true, update)
			}
		}
	}()
}
