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

package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cuelang.org/go/cue"
	upstreamcuex "github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/cue/process"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	apitypes "github.com/oam-dev/kubevela/apis/types"
	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/cue/render"
	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// ResolveSourceExpressions substitutes $(...) expressions in a properties blob.
//
// surface names the call site, which decides both what a source may read from
// context and - once the compatibility check lands - whether it may be consumed
// here at all.
func ResolveSourceExpressions(ctx process.Context, params interface{}, surface string) (interface{}, error) {
	if params == nil {
		return nil, nil
	}
	// The gate is read here rather than at each of the four render surfaces, so
	// they cannot drift: a surface that forgot to check would read $(VAR) as an
	// expression on a cluster where the feature is off, which is the failure the
	// gate exists to prevent.
	//
	// Returning params rather than the normalised copy matters. With the feature
	// off this function is a no-op, and a no-op that quietly round-tripped every
	// component's properties through JSON would still turn an int64 into a
	// float64.
	if !ExpressionsEnabledFor(appAnnotationsFrom(ctx)) {
		return params, nil
	}
	bt, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var normalized interface{}
	if err := json.Unmarshal(bt, &normalized); err != nil {
		return nil, err
	}

	// The Application render is one caller of the engine, not the owner of the
	// machinery. Everything specific to it - reading the pushed inputs, flattening
	// the render context, pushing the statuses back - lives here and nowhere else.
	in := sourceInputsFromContext(ctx)
	engine, err := NewSourceEngine(SourceEngineOptions{
		Surface:   surface,
		Context:   contextValuesFor(ctx),
		Bindings:  in.Bindings,
		Types:     in.Types,
		Templates: in.Templates,
		Sensitive: in.Sensitive,
		Store:     in.Store,
	})
	if err != nil {
		return nil, err
	}
	res, err := engine.Resolve(ctx.GetCtx(), normalized)
	if len(res.Statuses) > 0 {
		prior, _ := ctx.GetData(SourceResolutionStatusKey).(map[string]SourceResolutionStatus)
		ctx.PushData(SourceResolutionStatusKey, mergeStatuses(prior, res.Statuses))
	}
	if err != nil {
		return nil, err
	}
	return res.Properties, nil
}

// expressionContext pulls the fields this surface declares readable out of
// the render's process context.
func (r *sourceResolver) expressionContext() map[string]interface{} {
	out := map[string]interface{}{}
	for _, field := range propexpr.ContextFor(r.surface).ReadableFields() {
		if v := r.ctxValues[field]; v != nil {
			out[field] = v
		}
	}
	return out
}

type sourceResolver struct {
	// goCtx carries deadlines and cancellation into the fetches a template
	// performs.
	goCtx context.Context
	// ctxValues is the render context an expression and a template may read,
	// already narrowed to what the rules allow. A map rather than a
	// process.Context because reading field values is all either ever did, and
	// requiring the render's own context type would put this feature out of reach
	// of anything that is not an Application render.
	ctxValues map[string]interface{}
	// statuses accumulates what resolved, for the caller to report. Returned
	// rather than pushed back onto a context, so a caller that has no context to
	// push onto still gets it.
	statuses map[string]SourceResolutionStatus
	// readerKind and readerName name the thing currently doing the reading when it
	// is not the surface being rendered. Set while a chained source resolves its
	// own properties, empty otherwise.
	readerKind string
	readerName string
	// surface is the call site this resolver is working on behalf of. It decides
	// what a source may read from context: a chained source resolves inside
	// whichever render triggered the outer binding, so it inherits this too.
	surface         string
	sourceProps     map[string]map[string]interface{}
	sourceTypes     map[string]string
	sourceTemplates map[string]string
	sourceSchemas   map[string]string
	sensitivePaths  map[string][]string
	cacheStore      velaprocess.SourceCacheStore
	compiler        SourceCompiler
	resolved        map[string]map[string]interface{}
	resolving       map[string]bool
}

// SourceResolutionStatus captures source runtime resolution result.
const (
	// SourceResolutionStatusKey stores per-source runtime resolution statuses in
	// the render context, for a caller that has one.
	SourceResolutionStatusKey = "sourceResolutionStatuses"

	sourceCacheTTL            = 15 * time.Minute
	sourceCacheSyncAtKey      = apitypes.AnnotationConfigLastSyncAt
	sourceCacheAccessedKey    = apitypes.AnnotationConfigLastAccessed
	sourceCacheTTLKey         = apitypes.AnnotationConfigTTL
	sourceCacheTemplateKey    = apitypes.AnnotationConfigTemplate
	sourceCacheDataKey        = "input-properties"
	sourceCachePolicyUseStale = "use-stale"
	sourceCachePolicyFail     = "fail"
)

// CacheNamespace is where cache entries live: with the definitions they are
// bookkeeping about, which is the namespace the controller was told to use
// rather than a fixed "vela-system". An install with a non-default
// systemDefinitionNamespace would otherwise write to a namespace its own chart
// granted it no access to, and every cache write, touch and sweep would be
// Forbidden.
//
// Read through a function rather than copied into a constant: the flag that
// sets it is parsed after package initialisation.
func CacheNamespace() string {
	return oam.SystemDefinitionNamespace
}

// SourceCompiler evaluates a source's CUE template. Satisfied by
// *cuex.Compiler.
//
// An interface rather than the package singleton so a caller can supply its own
// provider set, and so a test can compile without reaching for global state. The
// resolver needs both methods: the template is evaluated with providers, and the
// storage block deliberately without them.
type SourceCompiler interface {
	CompileString(ctx context.Context, src string) (cue.Value, error)
	CompileStringWithOptions(ctx context.Context, src string, opts ...upstreamcuex.CompileOption) (cue.Value, error)
}

// sourceInputs is everything a resolution needs that is not the properties being
// resolved: which bindings exist, what definition backs each, and where values
// are cached.
//
// Explicit rather than pulled from the render context. The Application controller
// pushes these onto process.Context for its own reasons, but a resolver should
// not have to know that protocol to be usable - and a second caller has no
// process.Context to push onto.
type sourceInputs struct {
	// Bindings is spec.sources[].name -> that binding's own properties.
	Bindings map[string]map[string]interface{}
	// Types is binding name -> SourceDefinition type, carrying the pinned
	// revision where one was requested.
	Types map[string]string
	// Templates is definition type -> its CUE template.
	Templates map[string]string
	// Sensitive is definition type -> the paths its schema marks +sensitive.
	Sensitive map[string][]string
	// Store persists resolved values. Nil disables caching, which resolves
	// correctly and simply re-fetches.
	Store velaprocess.SourceCacheStore
	// Compiler evaluates source templates. Nil takes the workload compiler, which
	// is what the Application render has always used.
	Compiler SourceCompiler
}

// contextValuesFor flattens the render context into the field values a source may
// read, which is every field the cache-key rules allow. Narrowing to a surface
// happens later, when the source's own context block is rendered.
// appAnnotationsFrom reads the Application's annotations off the render context.
// Every process.Context carries them - NewContext pushes appAnnotations
// unconditionally - so this is the one thing a render knows about the
// Application it belongs to without being handed it.
func appAnnotationsFrom(ctx process.Context) map[string]string {
	switch v := ctx.GetData(velaprocess.ContextAppAnnotations).(type) {
	case map[string]string:
		return v
	case map[string]interface{}:
		out := make(map[string]string, len(v))
		for k, raw := range v {
			if s, ok := raw.(string); ok {
				out[k] = s
			}
		}
		return out
	}
	return nil
}

func contextValuesFor(ctx process.Context) map[string]interface{} {
	rules, err := cachekey.LoadRules()
	if err != nil {
		klog.Warningf("loading cache key rules for source context: %v", err)
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	for _, field := range rules.Fields() {
		if v := ctx.GetData(field); v != nil {
			out[field] = v
		}
	}
	// Readable context is a superset of keyed context: a surface may offer fields
	// an expression can read but a key may not be built from.
	for _, surface := range propexpr.SurfaceNames() {
		for _, field := range propexpr.ContextFor(surface).ReadableFields() {
			if _, have := out[field]; have {
				continue
			}
			if v := ctx.GetData(field); v != nil {
				out[field] = v
			}
		}
	}
	return out
}

// sourceInputsFromContext reads what the Application controller pushed. It is the
// bridge from the render context's protocol to explicit inputs, and the only
// place that protocol is understood.
func sourceInputsFromContext(ctx process.Context) sourceInputs {
	in := sourceInputs{
		Bindings:  map[string]map[string]interface{}{},
		Types:     map[string]string{},
		Templates: map[string]string{},
		Sensitive: map[string][]string{},
	}
	if v, ok := ctx.GetData(velaprocess.ContextAppSources).(map[string]map[string]interface{}); ok && v != nil {
		in.Bindings = v
	}
	if v, ok := ctx.GetData(velaprocess.ContextAppSourceTypes).(map[string]string); ok && v != nil {
		in.Types = v
	}
	if v, ok := ctx.GetData(velaprocess.ContextAppSourceTemplates).(map[string]string); ok && v != nil {
		in.Templates = v
	}
	if v, ok := ctx.GetData(velaprocess.ContextAppSourceSensitivePaths).(map[string][]string); ok && v != nil {
		in.Sensitive = v
	}
	if v, ok := ctx.GetData(velaprocess.ContextAppSourceCacheStore).(velaprocess.SourceCacheStore); ok && v != nil {
		in.Store = v
	}
	return in
}

// newSourceResolver builds a resolver for one render.
//
// ctx is still required, for three things that are genuinely the render's: the
// context values an expression may read, the Go context for I/O, and recording
// what resolved so status can report it.
func newSourceResolver(goCtx context.Context, ctxValues map[string]interface{}, surface string, in sourceInputs) *sourceResolver {
	// Schemas are derived from the templates rather than supplied: they are a
	// projection of the definition, so accepting them separately would allow the
	// two to disagree.
	sourceSchemas := map[string]string{}
	for sourceType, sourceTemplate := range in.Templates {
		schemaExpr, err := extractSourceSchemaExpr(sourceTemplate)
		if err != nil {
			klog.Warningf("extract source schema failed for %s: %v", sourceType, err)
			continue
		}
		if schemaExpr != "" {
			sourceSchemas[sourceType] = schemaExpr
		}
	}
	compiler := in.Compiler
	if compiler == nil {
		compiler = velacuex.SourceCompiler.Get()
	}
	return &sourceResolver{
		surface:         surface,
		goCtx:           goCtx,
		ctxValues:       ctxValues,
		statuses:        map[string]SourceResolutionStatus{},
		compiler:        compiler,
		sourceProps:     in.Bindings,
		sourceTypes:     in.Types,
		sourceTemplates: in.Templates,
		sourceSchemas:   sourceSchemas,
		sensitivePaths:  in.Sensitive,
		cacheStore:      in.Store,
		resolved:        map[string]map[string]interface{}{},
		resolving:       map[string]bool{},
	}
}

// resolve returns a binding's value, resolving it if this render has not
// already, and memoising the result for the rest of the render.
func (r *sourceResolver) resolve(sourceName string) (map[string]interface{}, error) {
	if v, ok := r.resolved[sourceName]; ok {
		return v, nil
	}
	if r.resolving[sourceName] {
		err := fmt.Errorf("circular source dependency detected at %q", sourceName)
		return r.fail(sourceName, "", "", err, "")
	}
	r.resolving[sourceName] = true
	defer delete(r.resolving, sourceName)

	sourceType, ok := r.sourceTypes[sourceName]
	if !ok || sourceType == "" {
		err := fmt.Errorf("source %q not found", sourceName)
		return r.fail(sourceName, "", "", err, "")
	}
	sourceTemplate, ok := r.sourceTemplates[sourceType]
	if !ok || sourceTemplate == "" {
		err := fmt.Errorf("source definition %q for source %q is missing cue template", sourceType, sourceName)
		return r.fail(sourceName, sourceType, "", err, "")
	}
	resolvedProps := map[string]interface{}{}
	paramFile := velaprocess.ParameterFieldName + ": {}"
	if props, ok := r.sourceProps[sourceName]; ok && props != nil {
		// A source's own properties may read an earlier source. Those reads belong
		// to this binding, not to the component whose render happened to trigger
		// the chain - without this the chain is invisible and the reads look like
		// the component made them directly.
		prevKind, prevName := r.readerKind, r.readerName
		r.readerKind, r.readerName = "source", sourceName
		resolvedPropsNode, err := resolveSourceNode(props, r)
		r.readerKind, r.readerName = prevKind, prevName
		if err != nil {
			r.setSourceStatus(sourceName, sourceType, PhaseFailed, err.Error(), "", "")
			return nil, errors.WithMessagef(err, "resolve source properties for %s", sourceName)
		}
		rp, ok := resolvedPropsNode.(map[string]interface{})
		if !ok {
			err := fmt.Errorf("resolved source properties for %s are invalid", sourceName)
			r.setSourceStatus(sourceName, sourceType, PhaseFailed, err.Error(), "", "")
			return nil, err
		}
		resolvedProps = rp
		raw, err := json.Marshal(rp)
		if err != nil {
			r.setSourceStatus(sourceName, sourceType, PhaseFailed, err.Error(), "", "")
			return nil, errors.WithMessagef(err, "marshal properties for source %s", sourceName)
		}
		paramFile = fmt.Sprintf("%s: %s", velaprocess.ParameterFieldName, string(raw))
	}
	cachePolicy, err := r.resolveCachePolicy(sourceName, sourceType, sourceTemplate, resolvedProps)
	if err != nil {
		return r.fail(sourceName, sourceType, "", err, "")
	}
	// storage.key is the readable prefix; uniqueness comes from the hash below,
	// which covers the definition's template, the binding's properties, and
	// exactly the context values the template reads.
	identity := identityInputs{
		Template:   templateFingerprint(sourceTemplate),
		Properties: resolvedProps,
		Context:    identityContext(r.ctxValues, sourceName, cachePolicy.KeyInputs),
	}
	cachePolicy.Key, err = cacheIdentity(cachePolicy.Key, identity)
	if err != nil {
		return r.fail(sourceName, sourceType, "", err, "")
	}
	cached, stale, found, cacheExpiresAt, err := r.readSourceCache(cachePolicy.Key, cachePolicy.TTL)
	if err != nil {
		klog.Warningf("read source cache failed for %s: %v", sourceName, err)
	} else if found {
		// A stored value has been through JSON. Retyping it against the schema
		// is what makes a cache hit and a fresh resolution mean the same thing
		// to an expression.
		cached = r.typeToSchema(sourceTemplate, cached)
		if !stale {
			r.resolved[sourceName] = cached
			r.setSourceStatus(sourceName, sourceType, PhaseResolved, "", cachePolicy.Key, formatExpiry(cacheExpiresAt))
			return cached, nil
		}
	}
	// What a failed refresh below may fall back on.
	fallback := staleFallback{
		name: sourceName, sourceType: sourceType, policy: cachePolicy,
		cached: cached, found: found, stale: stale, expiresAt: cacheExpiresAt,
	}

	// A source is compiled against the context the cache-key rules make readable,
	// not the component's - so it cannot depend on anything the key ignores.
	c, err := sourceContext(r.ctxValues, sourceName, r.surface)
	if err != nil {
		return r.fail(sourceName, sourceType, cachePolicy.Key, err, "")
	}
	val, err := r.compiler.CompileString(r.goCtx, strings.Join([]string{
		render.Template(sourceTemplate), paramFile, c,
	}, "\n"))
	if err != nil {
		if v, ok := r.serveStale(fallback, "refresh failed; serving stale cached value"); ok {
			return v, nil
		}
		return r.fail(sourceName, sourceType, cachePolicy.Key, err,
			fmt.Sprintf("compile source definition %s", sourceType))
	}
	if userErrs := render.UserErrors(val, "source definition", sourceType); len(userErrs) > 0 {
		errMsg := strings.Join(userErrs, "; ")
		if v, ok := r.serveStale(fallback, "refresh reported errors; serving stale cached value"); ok {
			return v, nil
		}
		return r.fail(sourceName, sourceType, cachePolicy.Key, fmt.Errorf("source definition %s reported errors: %s", sourceType, errMsg), "")
	}
	output := map[string]interface{}{}
	if err := val.LookupPath(value.FieldPath(velaprocess.OutputFieldName)).Decode(&output); err != nil {
		if v, ok := r.serveStale(fallback, "refresh failed; serving stale cached value"); ok {
			return v, nil
		}
		return r.fail(sourceName, sourceType, cachePolicy.Key, err,
			fmt.Sprintf("decode output for source definition %s", sourceType))
	}
	if err := r.validateResolvedOutput(sourceType, sourceTemplate, output); err != nil {
		if v, ok := r.serveStale(fallback, "refresh failed; serving stale cached value"); ok {
			return v, nil
		}
		return r.fail(sourceName, sourceType, cachePolicy.Key, err,
			fmt.Sprintf("validate output against schema for source definition %s", sourceType))
	}
	r.resolved[sourceName] = output
	expiresAt := time.Now().Add(cachePolicy.TTL).Format(time.RFC3339)
	if err := r.writeSourceCache(cachePolicy.Key, sourceType, output, cachePolicy.TTL,
		cachePolicy.KeyInputs, identity); err != nil {
		klog.Warningf("write source cache failed for %s: %v", sourceName, err)
	}
	r.setSourceStatus(sourceName, sourceType, PhaseResolved, "", cachePolicy.Key, expiresAt)
	return output, nil
}
