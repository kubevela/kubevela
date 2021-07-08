package process

import (
	"github.com/oam-dev/kubevela/pkg/cue/model"
	v1 "k8s.io/api/core/v1"
)

// workspace==(app,env)
type Workspace struct {
	app string
	env string
	triggerSource string
	schemas map[string]model.Instance
	preStore *v1.ConfigMap
	store *v1.ConfigMap
}

type PipelineContext struct {
	raw string
	job string
}

/*
artifact:
  build:
    type: configMap
tasks:
- name: gitops
  type: gitops
  parameter:
     url: git@...
     secret: git-secret
  upload: build
- name: testing-env
  type: scope
  downloadFrom: build
  tasks:
  - name: rendering
    type: traits
    input: ${build.test}
    parameter:

  - name: publish
  export: false
- name: prod-env
  type: scope
  tasks:
  - name: publish



scopes:
- name: build
  tasks:
  - name: helm-git
    parameter:
      url: git@...
      secret: git-secret
      outConfigmap: "xxx"
- name: test-env
  dataSource: "xxx"
  tasks:
  - name: workload
    input: ${apps.test}
*/

type TaskSpec struct {
    Env string
	InputPath string
	Template string
}

type Trigger struct {
	template string
	src string
}
