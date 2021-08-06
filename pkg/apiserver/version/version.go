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

package version

import "fmt"

const unspecified = "unspecified"

var (
	gitCommit = unspecified
	buildDate = unspecified
	version   = unspecified
)

// Info for git information
type Info struct {
	Version   string
	GitCommit string
	BuildDate string
}

// Get get info for git
func Get() Info {
	i := Info{
		Version:   version,
		GitCommit: gitCommit,
		BuildDate: buildDate,
	}
	if i.Version == "{version}" {
		i.Version = unspecified
	}
	if i.GitCommit == "{gitCommit}" {
		i.GitCommit = unspecified
	}
	if i.BuildDate == "{buildDate}" {
		i.BuildDate = unspecified
	}
	return i
}

func (i Info) String() string {
	return fmt.Sprintf(
		"Version: %s, GitCommit: %s, BuildDate: %s",
		i.Version,
		i.GitCommit,
		i.BuildDate,
	)
}
