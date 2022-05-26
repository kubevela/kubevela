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

package utils

import (
	"context"
)

type contextKey int

const (
	projectKey contextKey = iota
	usernameKey
)

// WithProject carries project in context
func WithProject(parent context.Context, project string) context.Context {
	return context.WithValue(parent, projectKey, project)
}

// ProjectFrom extract project from context
func ProjectFrom(ctx context.Context) (string, bool) {
	project, ok := ctx.Value(projectKey).(string)
	return project, ok
}

// WithUsername carries username in context
func WithUsername(parent context.Context, username string) context.Context {
	return context.WithValue(parent, usernameKey, username)
}

// UsernameFrom extract username from context
func UsernameFrom(ctx context.Context) (string, bool) {
	username, ok := ctx.Value(usernameKey).(string)
	return username, ok
}
