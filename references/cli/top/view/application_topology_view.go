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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// TopologyView display the resource topology of application
type TopologyView struct {
	*tview.Grid
	app           *App
	actions       model.KeyActions
	ctx           context.Context
	focusTopology bool
}

type topologyTree struct {
	*tview.TreeView
}

var (
	topologyViewInstance     = new(TopologyView)
	appTopologyInstance      = new(topologyTree)
	resourceTopologyInstance = new(topologyTree)
)

// NewTopologyView return a new topology view
func NewTopologyView(ctx context.Context, app *App) model.View {
	topologyViewInstance.app = app
	topologyViewInstance.ctx = ctx

	if topologyViewInstance.Grid == nil {
		topologyViewInstance.Grid = tview.NewGrid()
		topologyViewInstance.actions = make(model.KeyActions)
		topologyViewInstance.Init()
	}
	return topologyViewInstance
}

// Init the topology view
func (v *TopologyView) Init() {
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetRows(0).SetColumns(-1, -1)
	v.SetBorder(true)
	v.SetBorderAttributes(tcell.AttrItalic)
	v.SetTitle(title).SetTitleColor(config.ResourceTableTitleColor)
	v.bindKeys()
	v.SetInputCapture(v.keyboard)
}

// Start the topology view
func (v *TopologyView) Start() {
	appTopology := v.NewAppTopologyView()
	resourceTopology := v.NewResourceTopologyView()

	v.Grid.AddItem(appTopology, 0, 0, 1, 1, 0, 0, true)
	v.Grid.AddItem(resourceTopology, 0, 1, 1, 1, 0, 0, true)

	v.app.SetFocus(appTopology)
}

// Stop the topology view
func (v *TopologyView) Stop() {
	v.Grid.Clear()
}

// Hint return the menu hints of topology view
func (v *TopologyView) Hint() []model.MenuHint {
	return v.actions.Hint()
}

// Name return the name of topology view
func (v *TopologyView) Name() string {
	return "Topology"
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

// NewResourceTopologyView return a new resource topology view
func (v *TopologyView) NewResourceTopologyView() tview.Primitive {
	if resourceTopologyInstance.TreeView == nil {
		resourceTopologyInstance.TreeView = tview.NewTreeView()
		resourceTopologyInstance.SetGraphics(true)
		resourceTopologyInstance.SetGraphicsColor(tcell.ColorCadetBlue)
		resourceTopologyInstance.SetBorder(true)
		resourceTopologyInstance.SetTitle(fmt.Sprintf("[ %s ]", "Resource"))
	}

	appName := v.ctx.Value(&model.CtxKeyAppName).(string)
	namespace := v.ctx.Value(&model.CtxKeyNamespace).(string)

	root := tview.NewTreeNode(component.EmojiFormat(fmt.Sprintf("%s (%s)", appName, namespace), "app")).SetSelectable(true)
	resourceTopologyInstance.SetRoot(root)

	resourceTree, err := model.ApplicationResourceTopology(v.app.client, appName, namespace)
	if err != nil {
		return resourceTopologyInstance
	}
	for _, resource := range resourceTree {
		root.AddChild(buildTopology(resource.ResourceTree))
	}

	return resourceTopologyInstance
}

// NewAppTopologyView return a new app topology view
func (v *TopologyView) NewAppTopologyView() tview.Primitive {
	if appTopologyInstance.TreeView == nil {
		appTopologyInstance.TreeView = tview.NewTreeView()
		appTopologyInstance.SetGraphics(true)
		appTopologyInstance.SetGraphicsColor(tcell.ColorCadetBlue)
		appTopologyInstance.SetBorder(true)
		appTopologyInstance.SetTitle(fmt.Sprintf("[ %s ]", "App"))
	}

	appName := v.ctx.Value(&model.CtxKeyAppName).(string)
	namespace := v.ctx.Value(&model.CtxKeyNamespace).(string)

	root := tview.NewTreeNode(component.EmojiFormat(fmt.Sprintf("%s (%s)", appName, namespace), "app")).SetSelectable(true)

	appTopologyInstance.SetRoot(root)

	app, err := model.LoadApplication(v.app.client, appName, namespace)
	if err != nil {
		return appTopologyInstance
	}

	// workflow
	workflowNode := tview.NewTreeNode(component.EmojiFormat("WorkFlow", "workflow")).SetSelectable(true)
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
	componentTitleNode := tview.NewTreeNode(component.EmojiFormat("Component", "component")).SetSelectable(true)
	root.AddChild(componentTitleNode)
	for _, c := range app.Spec.Components {
		cNode := tview.NewTreeNode(c.Name)
		attrNode := tview.NewTreeNode("Attributes")
		attrNode.AddChild(tview.NewTreeNode(fmt.Sprintf("Type: %s", c.Type)))
		cNode.AddChild(attrNode)

		if len(c.Traits) > 0 {
			traitTitleNode := tview.NewTreeNode(component.EmojiFormat("Trait", "trait")).SetSelectable(true)
			cNode.AddChild(traitTitleNode)
			for _, trait := range c.Traits {
				traitNode := tview.NewTreeNode(trait.Type)
				traitTitleNode.AddChild(traitNode)
			}
		}

		componentTitleNode.AddChild(cNode)
	}

	// policy
	policyNode := tview.NewTreeNode(component.EmojiFormat("Policy", "policy")).SetSelectable(true)
	root.AddChild(policyNode)
	for _, policy := range app.Spec.Policies {
		policyNode.AddChild(tview.NewTreeNode(policy.Name))
	}

	return appTopologyInstance
}

func (v *TopologyView) switchTopology(_ *tcell.EventKey) *tcell.EventKey {
	if v.focusTopology {
		v.app.SetFocus(appTopologyInstance)
	} else {
		v.app.SetFocus(resourceTopologyInstance)
	}
	v.focusTopology = !v.focusTopology
	return nil
}

func buildTopology(node *types.ResourceTreeNode) *tview.TreeNode {
	if node == nil {
		return tview.NewTreeNode("?")
	}
	rootNode := tview.NewTreeNode(component.EmojiFormat(node.Name, node.Kind)).SetSelectable(true)

	attrNode := tview.NewTreeNode("Attributes")
	attrNode.AddChild(tview.NewTreeNode(fmt.Sprintf("Kind: %s", component.ColorizeKind(node.Kind))))
	attrNode.AddChild(tview.NewTreeNode(fmt.Sprintf("API Version: %s", node.APIVersion)))
	attrNode.AddChild(tview.NewTreeNode(fmt.Sprintf("Namespace: %s", node.Namespace)))
	attrNode.AddChild(tview.NewTreeNode(fmt.Sprintf("Cluster: %s", node.Cluster)))
	attrNode.AddChild(tview.NewTreeNode(fmt.Sprintf("Status: %s", component.ColorizeStatus(node.HealthStatus.Status))))

	rootNode.AddChild(attrNode)
	if len(node.LeafNodes) > 0 {
		subNode := tview.NewTreeNode("Sub Resource")
		rootNode.AddChild(subNode)

		for _, sub := range node.LeafNodes {
			subNode.AddChild(buildTopology(sub))
		}
	}

	return rootNode
}
