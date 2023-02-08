/*
Copyright 2021 The KubeVela Authors.

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
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/fatih/color"
	"github.com/kubevela/pkg/multicluster"
	"github.com/pkg/errors"
	"github.com/wercker/stern/stern"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/oam-dev/kubevela/pkg/oam/util"
	velaerr "github.com/oam-dev/kubevela/pkg/utils/errors"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
)

// MutateOption defines the function pattern for mutate
type MutateOption func(object metav1.Object) error

// MergeOverrideLabels will merge the existing labels and override by the labels passed in
func MergeOverrideLabels(labels map[string]string) MutateOption {
	return func(object metav1.Object) error {
		util.AddLabels(object, labels)
		return nil
	}
}

// MergeOverrideAnnotations will merge the existing annotations and override by the annotations passed in
func MergeOverrideAnnotations(annotations map[string]string) MutateOption {
	return func(object metav1.Object) error {
		util.AddAnnotations(object, annotations)
		return nil
	}
}

// MergeNoConflictLabels will merge the existing labels with the labels passed in, it will report conflicts if exists
func MergeNoConflictLabels(labels map[string]string) MutateOption {
	return func(object metav1.Object) error {
		existingLabels := object.GetLabels()
		// check and fill the labels
		for k, v := range labels {
			ev, ok := existingLabels[k]
			if ok && ev != "" && ev != v {
				return fmt.Errorf("%s for object %s, key: %s, conflicts value: %s <-> %s", velaerr.LabelConflict, object.GetName(), k, ev, v)
			}
			existingLabels[k] = v
		}
		object.SetLabels(existingLabels)
		return nil
	}
}

// CreateOrUpdateNamespace will create a namespace if not exist, it will also update a namespace if exists
// It will report an error if the labels conflict while it will override the annotations
func CreateOrUpdateNamespace(ctx context.Context, kubeClient client.Client, name string, options ...MutateOption) error {
	ns, err := GetNamespace(ctx, kubeClient, name)
	switch {
	case err == nil:
		return PatchNamespace(ctx, kubeClient, ns, options...)
	case apierrors.IsNotFound(err):
		return CreateNamespace(ctx, kubeClient, name, options...)
	default:
		return err
	}
}

// PatchNamespace will patch a namespace
func PatchNamespace(ctx context.Context, kubeClient client.Client, ns *corev1.Namespace, options ...MutateOption) error {
	original := ns.DeepCopy()
	for _, op := range options {
		if err := op(ns); err != nil {
			return err
		}
	}
	return kubeClient.Patch(ctx, ns, client.MergeFrom(original))
}

// CreateNamespace will create a namespace with mutate option
func CreateNamespace(ctx context.Context, kubeClient client.Client, name string, options ...MutateOption) error {
	obj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.NamespaceSpec{},
	}
	for _, op := range options {
		if err := op(obj); err != nil {
			return err
		}
	}
	return kubeClient.Create(ctx, obj)
}

// GetNamespace will return a namespace with mutate option
func GetNamespace(ctx context.Context, kubeClient client.Client, name string) (*corev1.Namespace, error) {
	obj := &corev1.Namespace{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: name}, obj)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// UpdateNamespace will update a namespace with mutate option
func UpdateNamespace(ctx context.Context, kubeClient client.Client, name string, options ...MutateOption) error {
	var namespace corev1.Namespace
	err := kubeClient.Get(ctx, k8stypes.NamespacedName{Name: name}, &namespace)
	if err != nil {
		return err
	}
	for _, op := range options {
		if err = op(&namespace); err != nil {
			return err
		}
	}
	return kubeClient.Update(ctx, &namespace)
}

// GetServiceAccountSubjectFromConfig extract ServiceAccountName subject from token
func GetServiceAccountSubjectFromConfig(cfg *rest.Config) string {
	sub, _ := GetTokenSubject(cfg.BearerToken)
	return sub
}

// GetCertificateCommonNameAndOrganizationsFromConfig extract CommonName and Organizations from Certificate
func GetCertificateCommonNameAndOrganizationsFromConfig(cfg *rest.Config) (string, []string) {
	cert := cfg.CertData
	if len(cert) == 0 && cfg.CertFile != "" {
		cert, _ = os.ReadFile(cfg.CertFile)
	}
	name, _ := GetCertificateSubject(cert)
	if name == nil {
		return "", nil
	}
	return name.CommonName, name.Organization
}

// GetUserInfoFromConfig extract UserInfo from KubeConfig
func GetUserInfoFromConfig(cfg *rest.Config) *authv1.UserInfo {
	if sub := GetServiceAccountSubjectFromConfig(cfg); sub != "" {
		return &authv1.UserInfo{Username: sub}
	}
	if cn, orgs := GetCertificateCommonNameAndOrganizationsFromConfig(cfg); cn != "" {
		return &authv1.UserInfo{Username: cn, Groups: orgs}
	}
	return nil
}

// AutoSetSelfImpersonationInConfig set impersonate username and group to the identity in the original rest config
func AutoSetSelfImpersonationInConfig(cfg *rest.Config) {
	if userInfo := GetUserInfoFromConfig(cfg); userInfo != nil {
		cfg.Impersonate.UserName = userInfo.Username
		cfg.Impersonate.Groups = append(cfg.Impersonate.Groups, userInfo.Groups...)
	}
}

// CreateOrUpdate create or update a kubernetes object
func CreateOrUpdate(ctx context.Context, cli client.Client, obj client.Object) (controllerutil.OperationResult, error) {
	bs, err := json.Marshal(obj)
	if err != nil {
		return controllerutil.OperationResultNone, err
	}
	return controllerutil.CreateOrUpdate(ctx, cli, obj, func() error {
		createTimestamp := obj.GetCreationTimestamp()
		resourceVersion := obj.GetResourceVersion()
		deletionTimestamp := obj.GetDeletionTimestamp()
		generation := obj.GetGeneration()
		managedFields := obj.GetManagedFields()
		if e := json.Unmarshal(bs, obj); err != nil {
			return e
		}
		obj.SetCreationTimestamp(createTimestamp)
		obj.SetResourceVersion(resourceVersion)
		obj.SetDeletionTimestamp(deletionTimestamp)
		obj.SetGeneration(generation)
		obj.SetManagedFields(managedFields)
		return nil
	})
}

// EscapeResourceNameToLabelValue parse characters in resource name to label valid name
func EscapeResourceNameToLabelValue(resourceName string) string {
	return strings.ReplaceAll(resourceName, ":", "_")
}

// IsClusterScope check if the gvk is cluster scoped
func IsClusterScope(gvk schema.GroupVersionKind, mapper meta.RESTMapper) (bool, error) {
	mappings, err := mapper.RESTMappings(gvk.GroupKind(), gvk.Version)
	isClusterScope := len(mappings) > 0 && mappings[0].Scope.Name() == meta.RESTScopeNameRoot
	return isClusterScope, err
}

// GetPodsLogs get logs from pods
func GetPodsLogs(ctx context.Context, config *rest.Config, containerName string, selectPods []*querytypes.PodBase, tmpl string, logC chan<- string, tailLines *int64) error {
	if err := verifyPods(selectPods); err != nil {
		return err
	}
	podRegex := getPodRegex(selectPods)
	pods, err := regexp.Compile(podRegex)
	if err != nil {
		return fmt.Errorf("fail to compile '%s' for logs query", podRegex)
	}
	container := regexp.MustCompile(".*")
	if containerName != "" {
		container = regexp.MustCompile(containerName + ".*")
	}
	// These pods are from the same namespace, so we can use the first one to get the namespace
	namespace := selectPods[0].Metadata.Namespace
	selector := labels.NewSelector()
	// Only use the labels to select pod if query one pod's log. It is only used when query vela-core log
	if len(selectPods) == 1 {
		for k, v := range selectPods[0].Metadata.Labels {
			req, _ := labels.NewRequirement(k, selection.Equals, []string{v})
			if req != nil {
				selector = selector.Add(*req)
			}
		}
	}
	config.Wrap(multicluster.NewTransportWrapper())
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	added, removed, err := stern.Watch(ctx,
		clientSet.CoreV1().Pods(namespace),
		pods,
		container,
		nil,
		[]stern.ContainerState{stern.RUNNING, stern.TERMINATED},
		selector,
	)
	if err != nil {
		return err
	}
	tails := make(map[string]*stern.Tail)

	funs := map[string]interface{}{
		"json": func(in interface{}) (string, error) {
			b, err := json.Marshal(in)
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
		"color": func(color color.Color, text string) string {
			return color.SprintFunc()(text)
		},
	}
	template, err := template.New("log").Funcs(funs).Parse(tmpl)
	if err != nil {
		return errors.Wrap(err, "unable to parse template")
	}

	go func() {
		for p := range added {
			id := p.GetID()
			if tails[id] != nil {
				continue
			}
			// 48h
			dur, _ := time.ParseDuration("48h")
			tail := stern.NewTail(p.Namespace, p.Pod, p.Container, template, &stern.TailOptions{
				Timestamps:   true,
				SinceSeconds: int64(dur.Seconds()),
				Exclude:      nil,
				Include:      nil,
				Namespace:    false,
				TailLines:    tailLines, // default for all logs
			})
			tails[id] = tail

			tail.Start(ctx, clientSet.CoreV1().Pods(p.Namespace), logC)
		}
	}()

	go func() {
		for p := range removed {
			id := p.GetID()
			if tails[id] == nil {
				continue
			}
			tails[id].Close()
			delete(tails, id)
		}
	}()

	<-ctx.Done()
	close(logC)
	return nil
}

func getPodRegex(pods []*querytypes.PodBase) string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, fmt.Sprintf("(%s.*)", pod.Metadata.Name))
	}
	return strings.Join(podNames, "|")
}

func verifyPods(pods []*querytypes.PodBase) error {
	if len(pods) == 0 {
		return errors.New("no pods selected")
	}
	if len(pods) == 1 {
		return nil
	}
	namespace := pods[0].Metadata.Namespace
	for _, pod := range pods {
		if pod.Metadata.Namespace != namespace {
			return errors.New("cannot select pods from different namespaces")
		}
	}
	return nil
}

// FilterObjectsByFieldSelector supports all field queries per type
func FilterObjectsByFieldSelector(objects []runtime.Object, fieldSelector fields.Selector) []runtime.Object {
	filterFunc := createFieldFilter(fieldSelector)
	// selected matched ones
	var filtered []runtime.Object
	for _, object := range objects {
		selected := true
		if !filterFunc(object) {
			selected = false
		}

		if selected {
			filtered = append(filtered, object)
		}
	}
	return filtered
}

// FilterFunc return true if object contains field selector
type FilterFunc func(object runtime.Object) bool

// createFieldFilter return filterFunc
func createFieldFilter(selector fields.Selector) FilterFunc {
	return func(object runtime.Object) bool {
		return contains(&object, selector)
	}
}

// implement a generic query filter to support multiple field selectors
func contains(object *runtime.Object, fieldSelector fields.Selector) bool {
	// call the ParseSelector function of "k8s.io/apimachinery/pkg/fields/selector.go" to validate and parse the selector
	for _, requirement := range fieldSelector.Requirements() {
		var negative bool
		// supports '=', '==' and '!='.(e.g. ?fieldSelector=key1=value1,key2=value2)
		// fields.ParseSelector(FieldSelector) has handled the case where the operator is '==' and converted it to '=',
		// so case selection.DoubleEquals can be ignored here.
		switch requirement.Operator {
		case selection.NotEquals:
			negative = true
		case selection.Equals:
			negative = false
		default:
			return false
		}
		key := requirement.Field
		value := requirement.Value

		data, err := json.Marshal(object)
		if err != nil {
			return false
		}
		result := gjson.Get(string(data), key)
		if (negative && fmt.Sprintf("%v", result.String()) != value) ||
			(!negative && fmt.Sprintf("%v", result.String()) == value) {
			continue
		} else {
			return false
		}
	}
	return true
}
