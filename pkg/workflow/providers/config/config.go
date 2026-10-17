/*
Copyright 2023 The KubeVela Authors.

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

package config

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	wfprovidertypes "github.com/kubevela/workflow/pkg/providers/types"
	wftypes "github.com/kubevela/workflow/pkg/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

const (
	// ProviderName is provider name
	ProviderName = "config"
)

// ErrRequestInvalid means the request is invalid
var ErrRequestInvalid = errors.New("the request is in valid")

// CreateConfigProperties the request body for creating a config
type CreateConfigProperties struct {
	Name      string                 `json:"name"`
	Namespace string                 `json:"namespace"`
	Template  string                 `json:"template,omitempty"`
	Config    map[string]interface{} `json:"config"`
}

// CreateParams is the create params
type CreateParams = oamprovidertypes.Params[CreateConfigProperties]

// Deps is what the four config steps need from the workflow runtime. The factory's
// dispatcher decides where a written Config lives: in the Application controller it is the
// application's ResourceTracker, so a Config a step creates is garbage-collected with the
// Application, as the legacy Secret was. The client reads Config CRs and probes whether the
// kind is served; the Action tells the workflow to wait for the controller or to fail the
// step; AppLabels name the Application the step runs for (app.oam.dev/name and namespace),
// read to refuse a legacy config another Application dispatched and to tell its own Config
// apart; Delete is the Application's delete handler, which lets a Config it tracks go for
// good. The legacy provider has the same four steps under other names and shares this.
type Deps struct {
	Factory   config.Factory
	Client    client.Client
	Action    wftypes.Action
	AppLabels map[string]string
	Delete    wfprovidertypes.Deleter
}

// DepsOf picks the steps' dependencies out of the runtime params.
func DepsOf(rp oamprovidertypes.RuntimeParams) Deps {
	deps := Deps{Factory: rp.ConfigFactory, Client: rp.KubeClient, Action: rp.Action, AppLabels: rp.AppLabels}
	if rp.KubeHandlers != nil {
		deps.Delete = rp.KubeHandlers.Delete
	}
	return deps
}

// Create writes the config as a Config CR and waits for the controller to render it.
//
// The template is read from either backend and the properties validated against it by
// ParseConfig, so a bad property or a missing template fails the step at once, before
// anything is written. When the cluster does not serve the Config kind, the config is
// written the legacy way, a Secret from this pod, as the issue keeps that fallback. A
// velacore-config Secret of that name that no Config owns is a legacy config, this step's
// own from a run on an older controller or the CLI's; the controller refuses to adopt an
// unowned Secret, so it is adopted first through the migration library. The Config and,
// for a sensitive template, its companion properties Secret go through the factory's
// dispatcher, which claims what that adoption wrote and refuses anyone else's. The step
// then ends only once the Config is Available for this spec, fails
// when the controller reports Error, and asks the workflow to wait otherwise, so a later
// read-config finds the Secret the way it did when the write was synchronous.
func Create(ctx context.Context, deps Deps, ccp CreateConfigProperties) error {
	tmpl := config.ParseTemplateRef(ccp.Template)
	served := configKindServed(deps.Client)
	if served {
		if err := templateAllowed(ctx, deps, tmpl, ccp); err != nil {
			return err
		}
	}
	// The template is read with the controller's identity, as the Config controller reads it
	// to render; who may use it was decided above, before any of it is rendered.
	item, err := deps.Factory.ParseConfig(auth.ContextClearUserInfo(ctx), tmpl, config.Metadata{
		NamespacedName: config.NamespacedName{Name: ccp.Name, Namespace: ccp.Namespace},
		Properties:     ccp.Config,
	})
	if err != nil {
		return err
	}
	if !served {
		// no Config kind, so no Config webhook and no scope rule on this path either: the
		// legacy write as before
		klog.InfoS("the Config kind is not served; writing the config as a legacy secret", "config", ccp.Namespace+"/"+ccp.Name)
		return deps.Factory.CreateOrUpdateConfig(ctx, item, ccp.Namespace)
	}
	if err := adoptLegacyConfig(ctx, deps, ccp.Namespace, ccp.Name); err != nil {
		return err
	}
	cfg, properties, err := config.BuildConfigCR(config.ConfigCRSpec{
		Name: ccp.Name, Namespace: ccp.Namespace,
		Template:   &tmpl,
		Sensitive:  item.Template.Sensitive,
		Properties: ccp.Config,
	})
	if err != nil {
		return err
	}
	if err := deps.Factory.ApplyConfigCR(ctx, cfg, properties); err != nil {
		return err
	}
	return awaitRendered(ctx, deps, cfg)
}

// templateAllowed is the access rule of issue #7410 section 6 for the step, answered before the
// template is read for rendering, so a user is never shown a template they may not use. One
// read of the template's scope, as the controller. With a named user in the context (the
// AuthenticateApplication gate on) a namespace-scoped template elsewhere is reviewed for that
// user, as the Config webhook reviews their write; the reads that follow go through the
// controller's cache and would not ask. Without one the controller writes as itself, so the
// answer is the structural one, for the Application's namespace: the step's namespace property
// cannot widen it. A binding the template grandfathers needs no review.
func templateAllowed(ctx context.Context, deps Deps, tmpl config.NamespacedName, ccp CreateConfigProperties) error {
	if tmpl.Name == "" {
		return nil // a config without a template
	}
	asController := auth.ContextClearUserInfo(ctx)
	resolved, waiting, err := config.TemplateScopeOf(asController, deps.Client, &configv1alpha1.ConfigTemplateReference{Name: tmpl.Name, Namespace: tmpl.Namespace})
	if errors.Is(err, config.ErrTemplateNotFound) {
		return nil // ParseConfig says so, as before
	}
	if err != nil {
		return err
	}
	user, named := auth.NamedUserFrom(ctx)
	from := ccp.Namespace
	if appNS := deps.AppLabels[oam.LabelAppNamespace]; !named && appNS != "" {
		from = appNS
	}
	if waiting {
		return config.PendingTemplateError(resolved, tmpl.Namespace, tmpl.Name, from)
	}
	access := config.AccessToTemplate(resolved, from)
	if access.Review == nil || resolved.Grandfathers(client.ObjectKey{Namespace: ccp.Namespace, Name: ccp.Name}) {
		return nil
	}
	if !named {
		return fmt.Errorf("%s; this application runs with no user to ask for (the %s feature gate is off), so this step may use only a template in %s or a system one", access.Reason, features.AuthenticateApplication, from)
	}
	allowed, err := auth.UserCan(asController, deps.Client, auth.UserInfoOf(user), *access.Review)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("%s; %s may not", access.Reason, user.GetName())
	}
	return nil
}

// configKindServed is the kind probe over the runtime's client; with no client there is
// nothing to probe and the legacy path is the only one.
func configKindServed(cli client.Client) bool {
	return cli != nil && config.KindServed(cli.RESTMapper(), configv1alpha1.ConfigGroupVersionKind)
}

// adoptLegacyConfig turns a legacy config Secret of that name into a Config the step can
// update. A velacore-config Secret that no Config owns is this step's own from a run on an
// older controller or the CLI's; the controller refuses to adopt an unowned Secret, so it is
// adopted first through the migration library (owner reference, migrated-to annotation, a
// Config built from the Secret's data; the Secret and its outputs released from the
// application that dispatched them), the same adoption vela config migrate and the startup
// sweep do. Any outcome but adopted fails the step with the library's reason. A plain
// Secret of that name is left for the controller to refuse; an existing Config is left to
// the dispatcher, which claims a migration-created one and refuses anyone else's.
//
// A legacy Secret another Application dispatched is refused, as the 1.11 dispatcher
// refused it: adopting it would hand that Application's config to this one. The marks
// the dispatcher left on it say whose it is; one with no marks (the CLI's) is anyone's.
func adoptLegacyConfig(ctx context.Context, deps Deps, ns, name string) error {
	cli := deps.Client
	exists, err := config.ConfigCRExists(ctx, cli, ns, name)
	if err != nil || exists {
		return err
	}
	var secret corev1.Secret
	if err := cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &secret); err != nil {
		return client.IgnoreNotFound(err)
	}
	if secret.Labels[types.LabelConfigCatalog] != types.VelaCoreConfig {
		return nil
	}
	if owner := applicationOf(secret.Labels); owner != "" && owner != applicationOf(deps.AppLabels) {
		return fmt.Errorf("config %s/%s is a legacy config managed by application %s", ns, name, owner)
	}
	report, err := config.NewMigrator(cli).MigrateConfig(ctx, ns, name, config.MigrationOptions{})
	if err != nil {
		return fmt.Errorf("config %s/%s is a legacy config that could not be adopted: %w", ns, name, err)
	}
	for _, res := range report.Results {
		if res.Outcome != config.MigrationAdopted {
			return fmt.Errorf("config %s/%s is a legacy config that could not be adopted (%s): %s", ns, name, res.Outcome, res.Reason)
		}
	}
	klog.InfoS("adopted the legacy config before updating it from the workflow step", "config", ns+"/"+name)
	return nil
}

// applicationOf names the Application whose dispatcher marked an object, "namespace/name",
// from the owner labels the resource keeper stamps or the app labels the generator merges
// in; "" when neither is there.
func applicationOf(labels map[string]string) string {
	if labels[oam.LabelOwnerKind] == v1beta1.ApplicationKind && labels[oam.LabelOwnerName] != "" {
		return labels[oam.LabelOwnerNamespace] + "/" + labels[oam.LabelOwnerName]
	}
	if labels[oam.LabelAppName] != "" && labels[oam.LabelAppNamespace] != "" {
		return labels[oam.LabelAppNamespace] + "/" + labels[oam.LabelAppName]
	}
	return ""
}

// awaitRendered reads the Config back and decides the step: Available for this spec ends
// it, Error fails it with the controller's message, anything else asks the workflow to wait:
// no status yet, the previous spec's status, or a cache that has not seen the write, which
// shows as a spec other than the one just applied or an observedGeneration behind the
// generation. A status written before observedGeneration existed is taken at its word.
func awaitRendered(ctx context.Context, deps Deps, want *configv1alpha1.Config) error {
	ns, name := want.Namespace, want.Name
	var cfg configv1alpha1.Config
	if err := deps.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &cfg); err != nil {
		if apierrors.IsNotFound(err) {
			return waitFor(deps, ns, name)
		}
		return err
	}
	if !sameSpec(cfg.Spec, want.Spec) {
		return waitFor(deps, ns, name)
	}
	if cfg.Status.ObservedGeneration != 0 && cfg.Status.ObservedGeneration != cfg.Generation {
		return waitFor(deps, ns, name)
	}
	switch cfg.Status.Phase {
	case configv1alpha1.ConfigPhaseAvailable:
		return nil
	case configv1alpha1.ConfigPhaseError:
		return renderError(&cfg)
	default:
		return waitFor(deps, ns, name)
	}
}

// renderError is the controller's reason, as the step reports it.
func renderError(cfg *configv1alpha1.Config) error {
	return fmt.Errorf("config %s/%s: %s", cfg.Namespace, cfg.Name, cfg.Status.GetCondition(condition.TypeSynced).Message)
}

// sameSpec compares the fields BuildConfigCR sets, the properties by value: the server
// re-serialises what it stores.
func sameSpec(got, want configv1alpha1.ConfigSpec) bool {
	return got.Alias == want.Alias && got.Description == want.Description &&
		reflect.DeepEqual(got.TemplateRef, want.TemplateRef) && reflect.DeepEqual(got.PropertiesFrom, want.PropertiesFrom) &&
		sameJSON(got.Properties, want.Properties)
}

// sameJSON compares two raw documents by value, numbers as the digits written (a float64
// would round large integers together); what does not parse is compared by bytes.
func sameJSON(a, b *runtime.RawExtension) bool {
	av, aok := decodeJSON(a)
	bv, bok := decodeJSON(b)
	if aok && bok {
		return reflect.DeepEqual(av, bv)
	}
	return bytes.Equal(rawOf(a), rawOf(b))
}

func decodeJSON(r *runtime.RawExtension) (any, bool) {
	if r == nil {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(r.Raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	return v, true
}

func rawOf(r *runtime.RawExtension) []byte {
	if r == nil {
		return nil
	}
	return r.Raw
}

// waitFor asks the workflow to come back later. With no workflow to ask, the caller gets
// an error rather than a success the Secret does not back yet.
func waitFor(deps Deps, ns, name string) error {
	if deps.Action == nil {
		return fmt.Errorf("config %s/%s is not rendered yet and there is no workflow to wait for it", ns, name)
	}
	deps.Action.Wait(fmt.Sprintf("waiting for config %s/%s to be rendered", ns, name))
	return nil
}

// CreateConfig creates a config
func CreateConfig(ctx context.Context, params *CreateParams) (*any, error) {
	return nil, Create(ctx, DepsOf(params.RuntimeParams), params.Params)
}

// ReadReturnVars is the read return vars
type ReadReturnVars struct {
	Config map[string]any `json:"config"`
}

// ReadReturns is the read returns
type ReadReturns = oamprovidertypes.Returns[ReadReturnVars]

// Read returns the config's properties from its Secret, whichever backend wrote it. A
// Config of the name decides first: in Error, the step fails with the controller's reason;
// not yet Available for its current spec, the workflow is asked to wait, so a read never
// returns the properties of the spec before an edit, or ones a failed render left behind.
// A Secret an Available Config has not yet shown the cache is a wait too. With no Config
// (or no Config kind) the Secret is the legacy one and is read as it always was.
func Read(ctx context.Context, deps Deps, nn config.NamespacedName) (map[string]any, error) {
	crBacked := false
	if deps.Client != nil {
		var cfg configv1alpha1.Config
		switch err := deps.Client.Get(ctx, client.ObjectKey{Namespace: nn.Namespace, Name: nn.Name}, &cfg); {
		case err == nil:
			crBacked = true
			if cfg.Status.Phase == configv1alpha1.ConfigPhaseError {
				return nil, renderError(&cfg)
			}
			if cfg.Status.Phase != configv1alpha1.ConfigPhaseAvailable || (cfg.Status.ObservedGeneration != 0 && cfg.Status.ObservedGeneration != cfg.Generation) {
				return nil, waitFor(deps, nn.Namespace, nn.Name)
			}
		case !apierrors.IsNotFound(err) && !config.IsCRDTypeMissing(err):
			return nil, err
		}
	}
	content, err := deps.Factory.ReadConfig(ctx, nn.Namespace, nn.Name)
	if err != nil && crBacked && apierrors.IsNotFound(err) {
		return nil, waitFor(deps, nn.Namespace, nn.Name)
	}
	return content, err
}

// ReadConfig reads the config
func ReadConfig(ctx context.Context, params *oamprovidertypes.Params[config.NamespacedName]) (*ReadReturns, error) {
	content, err := Read(ctx, DepsOf(params.RuntimeParams), params.Params)
	if err != nil {
		return nil, err
	}
	return &ReadReturns{Returns: ReadReturnVars{Config: content}}, nil
}

// ListVars is the list vars
type ListVars struct {
	Namespace string `json:"namespace"`
	Template  string `json:"template"`
}

// ListReturnVars is the list return vars
type ListReturnVars struct {
	Configs []map[string]any `json:"configs"`
}

// ListReturns is the list returns
type ListReturns = oamprovidertypes.Returns[ListReturnVars]

// List returns every config of the template in the namespace once, legacy or CR-backed.
func List(ctx context.Context, deps Deps, vars ListVars) ([]map[string]any, error) {
	if vars.Template == "" || vars.Namespace == "" {
		return nil, ErrRequestInvalid
	}
	template := config.ParseTemplateRef(vars.Template).Name
	configs, err := deps.Factory.ListAllConfigs(ctx, vars.Namespace, template)
	if err != nil {
		return nil, err
	}
	contents := []map[string]any{}
	for _, c := range configs {
		contents = append(contents, map[string]any{
			"name":        c.Name,
			"alias":       c.Alias,
			"description": c.Description,
			"config":      c.Properties,
		})
	}
	return contents, nil
}

// ListConfig lists the config
func ListConfig(ctx context.Context, params *oamprovidertypes.Params[ListVars]) (*ListReturns, error) {
	contents, err := List(ctx, DepsOf(params.RuntimeParams), params.Params)
	if err != nil {
		return nil, err
	}
	return &ListReturns{Returns: ListReturnVars{Configs: contents}}, nil
}

// configStepResourceCreator is what the step names as the creator of what it deletes, as the
// kube steps do.
const configStepResourceCreator = "workflow"

// Delete removes the config whichever backend holds it. A Config this Application
// dispatched goes through the Application's delete handler: the resource keeper marks the
// tracker's record deleted and removes the object, so state-keep does not re-apply the
// recorded manifest and bring it back; the companion properties Secret of ours goes the
// same way. Anything else, another Application's or the CLI's Config, a legacy Secret, or
// a caller with no handler, takes the factory's delete, which routes on what holds the
// config.
func Delete(ctx context.Context, deps Deps, nn config.NamespacedName) error {
	if deps.Delete != nil && deps.Client != nil {
		var cfg configv1alpha1.Config
		switch err := deps.Client.Get(ctx, client.ObjectKey{Namespace: nn.Namespace, Name: nn.Name}, &cfg); {
		case err == nil:
			if owner := applicationOf(cfg.Labels); owner != "" && owner == applicationOf(deps.AppLabels) {
				return deleteThroughApplication(ctx, deps, &cfg)
			}
		case !apierrors.IsNotFound(err) && !config.IsCRDTypeMissing(err):
			return err
		}
	}
	return deps.Factory.DeleteConfig(ctx, nn.Namespace, nn.Name)
}

func deleteThroughApplication(ctx context.Context, deps Deps, cfg *configv1alpha1.Config) error {
	var objects []*unstructured.Unstructured
	if cfg.Spec.PropertiesFrom != nil {
		var properties corev1.Secret
		switch err := deps.Client.Get(ctx, client.ObjectKey{Namespace: cfg.Namespace, Name: cfg.Spec.PropertiesFrom.SecretRef.Name}, &properties); {
		case err == nil:
			if config.IsPropertiesSecretOf(&properties, cfg.Name) {
				objects = append(objects, identityOf(corev1.SchemeGroupVersion.WithKind("Secret"), properties.Namespace, properties.Name))
			}
		case !apierrors.IsNotFound(err):
			return err
		}
	}
	objects = append(objects, identityOf(configv1alpha1.ConfigGroupVersionKind, cfg.Namespace, cfg.Name))
	for _, obj := range objects {
		if err := deps.Delete(ctx, deps.Client, "", configStepResourceCreator, obj); err != nil {
			return fmt.Errorf("fail to delete %s %s/%s: %w", obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
		}
	}
	return nil
}

// identityOf names an object for the delete handler, which reads the live object itself.
func identityOf(gvk schema.GroupVersionKind, namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return obj
}

// DeleteConfig deletes a config
func DeleteConfig(ctx context.Context, params *oamprovidertypes.Params[config.NamespacedName]) (*any, error) {
	return nil, Delete(ctx, DepsOf(params.RuntimeParams), params.Params)
}

//go:embed config.cue
var template string

// GetTemplate returns the cue template.
func GetTemplate() string {
	return template
}

// GetProviders returns the cue providers.
func GetProviders() map[string]cuexruntime.ProviderFn {
	return map[string]cuexruntime.ProviderFn{
		"create": oamprovidertypes.GenericProviderFn[CreateConfigProperties, any](CreateConfig),
		"read":   oamprovidertypes.GenericProviderFn[config.NamespacedName, ReadReturns](ReadConfig),
		"list":   oamprovidertypes.GenericProviderFn[ListVars, ListReturns](ListConfig),
		"delete": oamprovidertypes.GenericProviderFn[config.NamespacedName, any](DeleteConfig),
	}
}
