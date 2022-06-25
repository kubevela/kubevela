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

package addon

import (
	"fmt"
	"regexp"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
)

// NewAddonError will return an
func NewAddonError(msg string) error {
	return errors.New(msg)
}

var (
	// ErrRenderCueTmpl is error when render addon's cue file
	ErrRenderCueTmpl = NewAddonError("fail to render cue tmpl")

	// ErrRateLimit means exceed GitHub access rate limit
	ErrRateLimit = NewAddonError("exceed github access rate limit")

	// ErrNotExist  means addon not exists
	ErrNotExist = NewAddonError("addon not exist")

	// ErrRegistryNotExist means registry not exists
	ErrRegistryNotExist = NewAddonError("registry does not exist")
)

// WrapErrRateLimit return ErrRateLimit if is the situation, or return error directly
func WrapErrRateLimit(err error) error {
	errRate := &github.RateLimitError{}
	if errors.As(err, &errRate) {
		return ErrRateLimit
	}
	return err
}

// VersionUnMatchError means addon system requirement cannot meet requirement
type VersionUnMatchError struct {
	err       error
	addonName string
	// userSelectedAddonVersion is the version of the addon which is selected to install by user
	userSelectedAddonVersion string
	// availableVersion is the newest available addon's version which suits system requirements
	availableVersion string
}

func (v VersionUnMatchError) Error() string {
	var err string
	if v.availableVersion != "" {
		err = fmt.Sprintf("fail to install %s version of %s, because %s.\nInstall %s(v%s) which is the latest version that suits current version requirements", v.userSelectedAddonVersion, v.addonName, v.err, v.addonName, v.availableVersion)
	} else {
		err = fmt.Sprintf("fail to install %s version of %s, because %s", v.userSelectedAddonVersion, v.addonName, v.err)
	}
	return err
}

// GetAvailableVersionTip will return the available version from the error
func GetAvailableVersionTip(err error) (string, error) {
	compileRegex := regexp.MustCompile(`fail to install.*\sInstall.*v(\d+\.\d+\.\d+).*`)
	matchRes := compileRegex.FindStringSubmatch(err.Error())
	if len(matchRes) > 2 {
		return matchRes[2], nil
	}
	return "", errors.New("fail to load available version data")

}
