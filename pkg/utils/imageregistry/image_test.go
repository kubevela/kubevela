package image

import (
	"github.com/stretchr/testify/assert"
	"testing"
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
				errMsg:  "image is empty",
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
				errMsg:  "image jfsdfjwfjwf not found as its tag",
			},
		},
		"invalid image registry": {
			args: args{
				image: "nginx1sf/jfsdfjwfjwf:fwefsfsjflwejfwjfoewfsffsfw",
			},
			want: want{
				existed: false,
				errMsg:  "image jfsdfjwfjwf not found as its tag fwefsfsjflwejfwjfoewfsffsfw is not existed",
			},
		},
		"not docker hub registry": {
			args: args{
				image: "abc.com/d/e:v0.1",
			},
			want: want{
				existed: false,
				errMsg:  "image doesn't exist as its registry abc.com is not supported yet",
			},
		},
		"private registry, invalid authentication": {
			args: args{
				image:    "abc.com/d/e:v0.1",
				username: "admin",
			},
			want: want{
				existed: false,
				errMsg:  "unsupported protocol scheme",
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
