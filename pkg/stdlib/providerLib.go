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

package stdlib

var (
	kubeFile = file{
		name: "kube.cue",
		path: "vela/op",
		content: `
kube: {

  #Apply: {
     #do: "apply"
     #provider: "kube"
     value: {...}
     ...
  }

  #Read: {
     #do: "read"
     #provider: "kube"
     value?: {...}
     ...
  }

}

`,
	}

	workspaceFile = file{
		name: "workspace.cue",
		path: "vela/op",
		content: `
ws: {

  #Load: {
    #do: "load"
    component?: string
    value?: {...}
    ...
  }

  #Export: {
    #do: "export"
    component: string
    value: _
 }

  #DoVar: {
    #do: "var"
    method: *"Get" | "Put"
    path: sting
    value?: _
  }

}

`,
	}

	httpFile = file{
		name: "http.cue",
		path: "vela/op",
		content: `
http: #Do: {
		#do: "do"
		#provider: "http"
		
		method: *"GET" | "POST" | "PUT" | "DELETE"
		url: string
		request?: {
			body:    string
			header:  [string]: string
			trailer: [string]: string
		}
		response: {
			body: string
			header?:  [string]: [...string]
			trailer?: [string]: [...string]
		}
		...
}
`,
	}

	dingTalkFile = file{
		name: "dingTalk.cue",
		path: "vela/op",
		content: `
dingDing: {
    #Message: {
        text?: *null | {
                content: string
        }
        msgtype: string
        link?:   *null | {
                text?:       string
                title?:      string
                messageUrl?: string
                picUrl?:     string
        }
        markdown?: *null | {
                text:  string
                title: string
        }
        at?: *null | {
                atMobiles?: *null | [...string]
                isAtAll?:   bool
        }
        actionCard?: *null | {
                text:           string
                title:          string
                hideAvatar:     string
                btnOrientation: string
                singleTitle:    string
                singleURL:      string
                btns:           *null | [...*null | {
                        title:     string
                        actionURL: string
                }]
        }
        feedCard?: *null | {
                links: *null | [...*null | {
                        text?:       string
                        title?:      string
                        messageUrl?: string
                        picUrl?:     string
                }]
    }
}
`,
	}
)
