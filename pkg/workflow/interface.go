/*Copyright 2021 The KubeVela Authors.

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

package workflow

import (
"context"
"cuelang.org/go/cue"
"cuelang.org/go/cuego"
"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
"github.com/oam-dev/kubevela/pkg/appfile"
"cuelang.org/go/cue/build"
corev1 "k8s.io/api/core/v1"
"k8s.io/apimachinery/pkg/runtime"

"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Workflow is used to execute the workflow steps of Application.
type Workflow interface {
	// ExecuteSteps executes the steps of an Application with given steps of rendered resources.
	// It returns done=true only if all steps are executed and succeeded.
	ExecuteSteps(ctx context.Context, appRevName string, steps []*unstructured.Unstructured) (done bool, err error)
}

// SucceededMessage is the data json-marshalled into the message of `workflow-progress` condition
// when its reason is `succeeded`.
type SucceededMessage struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

type TaskRunner func(ctx workflow.Context)(common.WorkflowStepStatus,*Operation,error)

type Operation struct {
	Suspend bool
	Terminated bool
}

type StepWorkload struct {
	workload *appfile.Workload
	spec *v1aplha2.WorkloadStep
}

func newTask(t *StepWorkload,td TaskDiscovery,pd provider)(TaskRunner,error){
	exec:=new(TaskExecutor)
	return func(ctx workflow.Context)(common.WorkflowStepStatus,error){
		exec.do()
	},nil
	panic("")

}

type Context interface {
	Clone() Context
	GetComponent(name string,label map[string]string) (Component,error)
	PatchComponent(name string, label map[string]string, patchContent string)error
	GetVar(name string,scope string)(Var,error)
	SetVar(name string,scope string,v Var) error
	Step()(string,int)
	Commit()error
}

type Var struct {
	tpy string
	value string
}

func NewContext(cm *corev1.ConfigMap)Context{}

type Value string

func (v Value)Raw()string{
	return string(v)
}

func (v Value)UnmarshalTo(x interface{})error{
	return nil
}

func (v Value)Compile(r cue.Runtime)error{
	return nil
}

func (v Value)Fill(x Value,paths ...string)error{
	return nil
}

type TaskGenerator func(params Value,td TaskDiscovery,pds Providers)(TaskRunner,error)

type TaskDiscovery struct {
	builtins map[string]TaskGenerator
	extended map[string]TaskGenerator
}

type Providers struct {
	m map[string]provider
}
type provider struct {
	name string
	handles map[string]func(in Value)(Value,error)
	//Apply(obj *unstructured.Unstructured)(*unstructured.Unstructured,error)
	//Read(obj *unstructured.Unstructured)(*unstructured.Unstructured,error)
}


type TaskExecutor struct {
	td TaskDiscovery
	pds Providers
}

type cueValue struct {
	v cue.Value
	r cue.Runtime
}

func(t *TaskExecutor)do(steps cueValue,ctx workflow.Context)error{
	steps.v.Lookup("path")
}



