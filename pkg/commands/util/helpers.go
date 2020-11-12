package util

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/oam-dev/kubevela/api/types"

	"github.com/AlecAivazis/survey/v2"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/spf13/cobra"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultErrorExitCode = 1
)

func Print(msg string) {
	if klog.V(2) {
		klog.FatalDepth(2, msg)
	}
	if len(msg) > 0 {
		// add newline if needed
		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}
		fmt.Fprint(os.Stderr, msg)
	}
}

func fatal(msg string, code int) {
	if klog.V(2) {
		klog.FatalDepth(2, msg)
	}
	if len(msg) > 0 {
		// add newline if needed
		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}
		fmt.Fprint(os.Stderr, msg)
	}
	os.Exit(code)
}

func CheckErr(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "error: ") {
		msg = fmt.Sprintf("error: %s", msg)
	}
	fatal(msg, DefaultErrorExitCode)
}

func GetComponent(ctx context.Context, c client.Client, componentName string, namespace string) (corev1alpha2.Component, error) {
	var component corev1alpha2.Component
	err := c.Get(ctx, client.ObjectKey{Name: componentName, Namespace: namespace}, &component)
	return component, err
}

func GetAPIVersionKindFromTrait(td corev1alpha2.TraitDefinition) (string, string) {
	return td.Annotations[types.AnnAPIVersion], td.Annotations[types.AnnKind]
}

func GetAPIVersionKindFromWorkload(td corev1alpha2.WorkloadDefinition) (string, string) {
	return td.Annotations[types.AnnAPIVersion], td.Annotations[types.AnnKind]
}

func PrintFlags(cmd *cobra.Command, subcmds []*cobra.Command) {
	cmd.Println("Flags:")
	for _, sub := range subcmds {
		if sub.HasLocalFlags() {
			cmd.Println(sub.LocalFlags().FlagUsages())
		}
	}
	cmd.Println()
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
		return "", fmt.Errorf("choosing service err %v", err)
	}
	return svcName, nil
}
