/*
Copyright 2026 The KubeVela Authors.

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

package sourcedefinition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"cuelang.org/go/cue/ast"
	cueformat "cuelang.org/go/cue/format"
	cueparser "cuelang.org/go/cue/parser"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	ctrlrec "github.com/kubevela/pkg/controller/reconciler"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/config"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	coredef "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/core"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/version"
)

const (
	sourceTemplateNamespace  = "vela-system"
	sourceTemplateNamePrefix = "source-"
	sourceTemplateHashLen    = 8
	maxK8sNameLen            = 63
)

// +kubebuilder:rbac:groups=core.oam.dev,resources=sourcedefinitions,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core.oam.dev,resources=sourcedefinitions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete

// Reconciler reconciles a SourceDefinition object.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
	record event.Recorder
	options
}

type options struct {
	concurrentReconciles int
	ignoreDefNoCtrlReq   bool
	controllerVersion    string
	cacheGCInterval      time.Duration
	cacheGCEnabled       bool
	defRevLimit          int
}

// defaultCacheGCInterval is how often the source cache/template GC sweep runs
// when no explicit interval is configured.
const defaultCacheGCInterval = 10 * time.Minute

// Reconcile is the main logic for SourceDefinition controller.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := ctrlrec.NewReconcileContext(ctx)
	defer cancel()

	klog.InfoS("Reconcile sourceDefinition", "sourceDefinition", klog.KRef(req.Namespace, req.Name))

	var sourceDefinition v1beta1.SourceDefinition
	if err := r.Get(ctx, req.NamespacedName, &sourceDefinition); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !coredef.MatchControllerRequirement(&sourceDefinition, r.controllerVersion, r.ignoreDefNoCtrlReq) {
		klog.InfoS("skip definition: not match the controller requirement of definition", "sourceDefinition", klog.KObj(&sourceDefinition))
		return ctrl.Result{}, nil
	}

	// Revision first, so the schema template below is stored against a revision
	// that already exists - the same order componentdefinition uses.
	_, result, err := coredef.ReconcileDefinitionRevision(ctx, r.Client, r.record, &sourceDefinition, r.defRevLimit,
		func(revision *common.Revision) error {
			sourceDefinition.Status.LatestRevision = revision
			return r.UpdateStatus(ctx, &sourceDefinition)
		})
	if result != nil {
		return *result, err
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	nextRef, err := r.reconcileSchemaTemplate(ctx, &sourceDefinition)
	if err != nil {
		r.record.Event(&sourceDefinition, event.Warning("cannot reconcile SourceDefinition schema template", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, &sourceDefinition,
			condition.ReconcileError(fmt.Errorf("failed to reconcile source schema template for %s: %w", sourceDefinition.Name, err)))
	}

	currentRef := sourceDefinition.Status.ConfigTemplateRef
	refChanged := (currentRef == nil) != (nextRef == nil)
	if !refChanged && currentRef != nil && nextRef != nil {
		refChanged = currentRef.Name != nextRef.Name || currentRef.SchemaHash != nextRef.SchemaHash
	}
	if refChanged {
		sourceDefinition.Status.ConfigTemplateRef = nextRef
		sourceDefinition.Status.Conditions = []condition.Condition{condition.ReconcileSuccess()}
		if err := r.UpdateStatus(ctx, &sourceDefinition); err != nil {
			return ctrl.Result{}, util.PatchCondition(ctx, r, &sourceDefinition,
				condition.ReconcileError(fmt.Errorf("failed to update SourceDefinition status for %s: %w", sourceDefinition.Name, err)))
		}
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileSchemaTemplate(ctx context.Context, def *v1beta1.SourceDefinition) (*v1beta1.SourceDefinitionConfigTemplateRef, error) {
	if def == nil || def.Spec.Schematic == nil || def.Spec.Schematic.CUE == nil || def.Spec.Schematic.CUE.Template == "" {
		return nil, nil
	}
	schemaExpr, err := extractSchemaExpr(def.Spec.Schematic.CUE.Template)
	if err != nil {
		return nil, err
	}
	if schemaExpr == "" {
		return nil, nil
	}

	rawHash := sha256.Sum256([]byte(schemaExpr))
	schemaHash := hex.EncodeToString(rawHash[:])
	templateName := buildSchemaTemplateName(def.Name, schemaHash)
	templateContent := fmt.Sprintf(`metadata: {
  name: "%s"
  alias: "%s"
  scope: "system"
  description: "Auto-generated source schema template for %s/%s (schemaHash: %s)"
}
template: {
  parameter: %s
  output: {}
}
`, templateName, def.Name, def.Namespace, def.Name, schemaHash, schemaExpr)

	factory := config.NewConfigFactory(r.Client)
	tmpl, err := factory.ParseTemplate(ctx, templateName, []byte(templateContent))
	if err != nil {
		return nil, err
	}
	// Stamp the owning SourceDefinition identity onto the ConfigTemplate as
	// queryable labels so the cache GC sweep can determine whether a template is
	// still referenced by a live SourceDefinition without parsing the CUE
	// description. Additive to the labels ParseTemplate already sets.
	if tmpl.ConfigMap != nil {
		if tmpl.ConfigMap.Labels == nil {
			tmpl.ConfigMap.Labels = map[string]string{}
		}
		tmpl.ConfigMap.Labels[apitypes.LabelSourceDefinitionName] = def.Name
		if def.Namespace != "" {
			tmpl.ConfigMap.Labels[apitypes.LabelSourceDefinitionNamespace] = def.Namespace
		}
	}
	if err := factory.CreateOrUpdateConfigTemplate(ctx, sourceTemplateNamespace, tmpl); err != nil {
		return nil, err
	}
	return &v1beta1.SourceDefinitionConfigTemplateRef{
		Name:       templateName,
		SchemaHash: schemaHash,
	}, nil
}

func extractSchemaExpr(template string) (string, error) {
	file, err := cueparser.ParseFile("-", template, cueparser.ParseComments)
	if err != nil {
		return "", err
	}
	for _, decl := range file.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		name, _, err := ast.LabelName(field.Label)
		if err != nil || name != "schema" {
			continue
		}
		bt, err := cueformat.Node(field.Value)
		if err != nil {
			return "", err
		}
		return string(bt), nil
	}
	return "", nil
}

func buildSchemaTemplateName(name, schemaHash string) string {
	shortHash := schemaHash
	if len(shortHash) > sourceTemplateHashLen {
		shortHash = shortHash[:sourceTemplateHashLen]
	}
	safeName := sanitizeName(name)
	if safeName == "" {
		safeName = "source"
	}
	suffix := "-" + shortHash
	maxNameLen := maxK8sNameLen - len(sourceTemplateNamePrefix) - len(suffix)
	if maxNameLen < 1 {
		maxNameLen = 1
	}
	if len(safeName) > maxNameLen {
		safeName = strings.Trim(safeName[:maxNameLen], "-")
		if safeName == "" {
			safeName = "source"
		}
	}
	return sourceTemplateNamePrefix + safeName + suffix
}

func sanitizeName(name string) string {
	s := strings.ToLower(name)
	var b strings.Builder
	b.Grow(len(s))
	lastDash := false
	for _, r := range s {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// UpdateStatus updates v1beta1.SourceDefinition's status with retry.RetryOnConflict.
func (r *Reconciler) UpdateStatus(ctx context.Context, def *v1beta1.SourceDefinition, opts ...client.SubResourceUpdateOption) error {
	status := def.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Get(ctx, client.ObjectKey{Namespace: def.Namespace, Name: def.Name}, def); err != nil {
			return
		}
		def.Status = status
		return r.Status().Update(ctx, def, opts...)
	})
}

// SetupWithManager will setup with event recorder.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("SourceDefinition")).
		WithAnnotations("controller", "SourceDefinition")
	if r.cacheGCEnabled {
		interval := r.cacheGCInterval
		if interval <= 0 {
			interval = defaultCacheGCInterval
		}
		if err := mgr.Add(&cacheGCRunnable{reconciler: r, interval: interval}); err != nil {
			return err
		}
	}
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		For(&v1beta1.SourceDefinition{}).
		Complete(r)
}

// cacheGCRunnable drives the periodic source cache/template GC sweep. It is a
// manager Runnable rather than a reconcile hook so the sweep runs on a fixed
// timer independent of individual SourceDefinition events, and stops cleanly
// when the manager's context is cancelled.
type cacheGCRunnable struct {
	reconciler *Reconciler
	interval   time.Duration
}

// Start runs the sweep on a ticker until ctx is done.
func (g *cacheGCRunnable) Start(ctx context.Context) error {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := g.reconciler.sweepSourceCache(ctx); err != nil {
				klog.ErrorS(err, "source cache GC sweep failed")
			}
		}
	}
}

// Setup adds a controller that reconciles SourceDefinition.
func Setup(mgr ctrl.Manager, args oamctrl.Args) error {
	r := Reconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		options: parseOptions(args),
	}
	return r.SetupWithManager(mgr)
}

func parseOptions(args oamctrl.Args) options {
	return options{
		concurrentReconciles: args.ConcurrentReconciles,
		ignoreDefNoCtrlReq:   args.IgnoreDefinitionWithoutControllerRequirement,
		controllerVersion:    version.VelaVersion,
		cacheGCInterval:      defaultCacheGCInterval,
		cacheGCEnabled:       true,
		defRevLimit:          args.DefRevisionLimit,
	}
}
