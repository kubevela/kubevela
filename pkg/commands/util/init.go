package util

import (
	"context"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// DoesNamespaceExist check namespace exist
func DoesNamespaceExist(c client.Client, namespace string) (bool, error) {
	var ns corev1.Namespace
	err := c.Get(context.Background(), types.NamespacedName{Name: namespace}, &ns)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func NewNamespace(c client.Client, namespace string) error {
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	err := c.Create(context.Background(), ns)
	if err != nil {
		return err
	}
	return nil
}

// DoesCoreCRDExist check CRD exist
func DoesCRDExist(cxt context.Context, c client.Client, crdName string) (bool, error) {
	err := c.Get(cxt, types.NamespacedName{Name:crdName}, &apiextensions.CustomResourceDefinition{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
