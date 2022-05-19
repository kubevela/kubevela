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

package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExisted(t *testing.T) {
	type args struct {
		username string
		password string
		image    string
	}
	type want struct {
		existed bool
		errMsg  string
	}
	testcases := map[string]struct {
		args args
		want want
	}{
		"empty image name": {
			args: args{
				image: "",
			},
			want: want{
				existed: false,
				errMsg:  "could not parse referenc",
			},
		},
		"just image name": {
			args: args{
				image: "nginx",
			},
			want: want{
				existed: true,
			},
		},
		" image name with registry": {
			args: args{
				image: "docker.io/nginx",
			},
			want: want{
				existed: true,
			},
		},
		"image name with tag": {
			args: args{
				image: "nginx:latest",
			},
			want: want{
				existed: true,
			},
		},
		"image name with repository": {
			args: args{
				image: "library/nginx:latest",
			},
			want: want{
				existed: true,
			},
		},
		"image name with registry and repository": {
			args: args{
				image: "docker.io/library/nginx",
			},
			want: want{
				existed: true,
			},
		},
		"invalid image name": {
			args: args{
				image: "jfsdfjwfjwf:fwefsfsjflwejfwjfoewfsffsfw",
			},
			want: want{
				existed: false,
				errMsg:  "UNAUTHORIZED",
			},
		},
		"invalid image registry": {
			args: args{
				image: "nginx1sf/jfsdfjwfjwf:fwefsfsjflwejfwjfoewfsffsfw",
			},
			want: want{
				existed: false,
				errMsg:  "UNAUTHORIZED",
			},
		},
		"registry is not valid": {
			args: args{
				image: "abcYeidlfdned877239.com/d/e:v0.1",
			},
			want: want{
				existed: false,
				errMsg:  "EOF",
			},
		},
		"not docker hub registry": {
			args: args{
				image: "alibabacloud.com/d/e:v0.1",
			},
			want: want{
				existed: false,
				errMsg:  "invalid character '<' looking for beginning of value",
			},
		},
		"private registry, invalid authentication": {
			args: args{
				image:    "abcfefsfjflsfjweffwe73rr.com/d/e:v0.1",
				username: "admin",
			},
			want: want{
				existed: false,
				errMsg:  "Get",
			},
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			got, err := IsExisted(tc.args.username, tc.args.password, tc.args.image)
			if err != nil || tc.want.errMsg != "" {
				assert.Contains(t, err.Error(), tc.want.errMsg)
			}
			assert.Equal(t, got, tc.want.existed)
		})
	}
}
