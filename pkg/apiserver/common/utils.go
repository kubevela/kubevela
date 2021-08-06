package common

import "github.com/oam-dev/kubevela/pkg/apiserver/proto/model"

// Reverse reverse properties in list
func Reverse(arr *[]*model.Properties) {
	length := len(*arr)
	for i := 0; i < length/2; i++ {
		(*arr)[i], (*arr)[length-1-i] = (*arr)[length-1-i], (*arr)[i]
	}
}
