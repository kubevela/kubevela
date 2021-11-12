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

package utils

import (
	"fmt"
	"regexp"
)

// ParseAPIServerEndpoint automatically construct the full url of APIServer
// It will patch port and scheme if not exists
func ParseAPIServerEndpoint(server string) (string, error) {
	r := regexp.MustCompile(`^((?P<scheme>http|https)://)?(?P<host>[^:\s]+)(:(?P<port>[0-9]+))?$`)
	if !r.MatchString(server) {
		return "", fmt.Errorf("invalid endpoint url: %s", server)
	}
	var scheme, port, host string
	results := r.FindStringSubmatch(server)
	for i, name := range r.SubexpNames() {
		switch name {
		case "scheme":
			scheme = results[i]
		case "host":
			host = results[i]
		case "port":
			port = results[i]
		}
	}
	if scheme == "" {
		if port == "80" {
			scheme = "http"
		} else {
			scheme = "https"
		}
	}
	if port == "" {
		if scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	return fmt.Sprintf("%s://%s:%s", scheme, host, port), nil
}
