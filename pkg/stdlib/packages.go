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

type discover struct {
	files []file
}

// Pkgs is map[${path}]${package-content}
type Pkgs map[string]string

func (p *discover) packages() Pkgs {
	pkgs := map[string]string{}
	for _, f := range p.files {
		pkgs[f.path] += f.content + "\n"
	}
	return pkgs
}

func (p *discover) addFile(f file) {
	p.files = append(p.files, f)
}

// GetPackages Get Stdlib packages
func GetPackages() Pkgs {
	d := &discover{}
	d.addFile(opFile)
	d.addFile(kubeFile)
	d.addFile(workspaceFile)
	d.addFile(httpFile)
	d.addFile(dingTalkFile)
	return d.packages()
}
