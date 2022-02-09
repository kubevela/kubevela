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

import "github.com/hashicorp/go-version"

// GitRevision is the commit of repo
var GitRevision = "UNKNOWN"

// VelaVersion is the version of cli.
var VelaVersion = "UNKNOWN"

// IsOfficialKubeVelaVersion checks whether the provided version string follows a KubeVela version pattern
func IsOfficialKubeVelaVersion(versionStr string) bool {
	_, err := version.NewSemver(versionStr)
	return err == nil
}

// GetOfficialKubeVelaVersion extracts the KubeVela version from the provided string
// More precisely, this method returns the segments and prerelease info w/o metadata
func GetOfficialKubeVelaVersion(versionStr string) (string, error) {
	s, err := version.NewSemver(versionStr)
	if err != nil {
		return "", err
	}
	v := s.String()
	metadata := s.Metadata()
	if metadata != "" {
		metadata = "+" + metadata
	}
	return v[:len(v)-len(metadata)], nil
}
