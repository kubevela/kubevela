package test

import (
	"github.com/kubevela/pkg/cue/cuex/providers"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type ResourceReturns providers.Returns[*unstructured.Unstructured]

type Any struct {
	A map[string]interface{} `json:"a"`
	B map[string]any         `json:"b"`
	C map[string]string      `json:"c"`
	D any                    `json:"d"`
}
