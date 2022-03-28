package utils

import "testing"

func TestGetPrivateImage(t *testing.T) {
	r := Registry{
		Registry: "registry.cn-beijing.aliyuncs.com",
		Username: "hybridcloud@prod.trusteeship.aliyunid.com",
		Password: "Alibaba1234",
	}
	r.GetPrivateImages()
}
