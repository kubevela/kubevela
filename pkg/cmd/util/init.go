package util

import (
	"context"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetKubeClient creates a Kubernetes config and client for a given kubeconfig context.
func GetKubeClient() error {

	config, err := clientcmd.BuildConfigFromFlags("", GetKubeConfig())
	if err != nil {
		return err
	}

	// Creates the dynamic interface.
	_, err = kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	return nil
}

func HomeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE")
}

func GetKubeConfig() string {
	return filepath.Join(HomeDir(), ".kube", "config")
}

func IsNamespaceExist(c client.Client, namespace string) bool {
	var ns corev1.Namespace
	err := c.Get(context.Background(), types.NamespacedName{Name: namespace}, &ns)
	return err == nil
}

func NewNamespace(c client.Client, namespace string) error {
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	err := c.Create(context.Background(), ns)
	if err != nil {
		return err
	}
	return nil
}

func IsCoreCRDExist(c client.Client, cxt context.Context, object runtime.Object) error {
	return c.List(cxt, object, &client.ListOptions{})
}
