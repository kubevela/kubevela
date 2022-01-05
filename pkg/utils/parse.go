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
	"net/url"
	"strings"

	"github.com/pkg/errors"
)

// TypeLocal represents github
const TypeLocal = "local"

// TypeOss represent oss
const TypeOss = "oss"

// TypeGithub represents github
const TypeGithub = "github"

// TypeUnknown represents parse failed
const TypeUnknown = "unknown"

// Content contains different type of content needed when building Registry
type Content struct {
	OssContent
	GithubContent
	LocalContent
}

// LocalContent for local registry
type LocalContent struct {
	AbsDir string `json:"abs_dir"`
}

// OssContent for oss registry
type OssContent struct {
	EndPoint string `json:"bucket_url"`
	Bucket   string `json:"bucket"`
}

// GithubContent for cap center
type GithubContent struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Path  string `json:"path"`
	Ref   string `json:"ref"`
}

// Parse will parse config from address
func Parse(addr string) (string, *Content, error) {
	URL, err := url.Parse(addr)
	if err != nil {
		return "", nil, err
	}
	l := strings.Split(strings.TrimPrefix(URL.Path, "/"), "/")
	switch URL.Scheme {
	case "http", "https":
		switch URL.Host {
		case "github.com":
			// We support two valid format:
			// 1. https://github.com/<owner>/<repo>/tree/<branch>/<path-to-dir>
			// 2. https://github.com/<owner>/<repo>/<path-to-dir>
			if len(l) < 3 {
				return "", nil, errors.New("invalid format " + addr)
			}
			if l[2] == "tree" {
				// https://github.com/<owner>/<repo>/tree/<branch>/<path-to-dir>
				if len(l) < 5 {
					return "", nil, errors.New("invalid format " + addr)
				}
				return TypeGithub, &Content{
					GithubContent: GithubContent{
						Owner: l[0],
						Repo:  l[1],
						Path:  strings.Join(l[4:], "/"),
						Ref:   l[3],
					},
				}, nil
			}
			// https://github.com/<owner>/<repo>/<path-to-dir>
			return TypeGithub, &Content{
					GithubContent: GithubContent{
						Owner: l[0],
						Repo:  l[1],
						Path:  strings.Join(l[2:], "/"),
						Ref:   "", // use default branch
					},
				},
				nil
		case "api.github.com":
			if len(l) != 5 {
				return "", nil, errors.New("invalid format " + addr)
			}
			//https://api.github.com/repos/<owner>/<repo>/contents/<path-to-dir>
			return TypeGithub, &Content{
					GithubContent: GithubContent{
						Owner: l[1],
						Repo:  l[2],
						Path:  l[4],
						Ref:   URL.Query().Get("ref"),
					},
				},
				nil
		default:
			return "", nil, fmt.Errorf("git type repository only support github for now")
		}
	case "oss":
		return TypeOss, &Content{
			OssContent: OssContent{
				EndPoint: URL.Host,
				Bucket:   URL.Path,
			},
		}, nil
	case "file":
		return TypeLocal, &Content{
			LocalContent: LocalContent{
				AbsDir: URL.Path,
			},
		}, nil

	}

	return TypeUnknown, nil, nil
}
