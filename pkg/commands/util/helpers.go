package util

import (
	"context"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetComponent get OAM component
func GetComponent(ctx context.Context, c client.Client, componentName string, namespace string) (corev1alpha2.Component, error) {
	var component corev1alpha2.Component
	err := c.Get(ctx, client.ObjectKey{Name: componentName, Namespace: namespace}, &component)
	return component, err
}

// AskToChooseOneService will ask users to select one service of the application if more than one exidi
func AskToChooseOneService(svcNames []string) (string, error) {
	if len(svcNames) == 0 {
		return "", fmt.Errorf("no service exist in the application")
	}
	if len(svcNames) == 1 {
		return svcNames[0], nil
	}
	prompt := &survey.Select{
		Message: "You have multiple services in your app. Please choose one service: ",
		Options: svcNames,
	}
	var svcName string
	err := survey.AskOne(prompt, &svcName)
	if err != nil {
		return "", fmt.Errorf("choosing service err %w", err)
	}
	return svcName, nil
}
