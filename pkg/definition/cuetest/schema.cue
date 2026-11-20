// Package test declares KubeVela definition test cases. Import it as
// "vela/test" and give each case its own top-level field:
//
//	"defaults to one replica": test.#ComponentRender & {
//		definition: "webservice"
//		parameter: image: "nginx"
//		expect: output: spec: template: spec: containers: [{image: "nginx"}]
//	}
//
// An expectation passes when it subsumes the result: fields it names must be
// present and match, `f?: _|_` asserts f is absent, and a list without `...`
// fixes its length. CUE constraints negate scalars (`!=3`, `!~"^nginx"`), and
// attributes on an expected field refine it:
//
//	f: {...} @exact()             no fields beyond those given
//	f: {...} @not()               passes unless f is present and matches
//	f: [...] @contains()          each element matches a different element, anywhere
//	f: [...] @contains() @not()   none of the elements is present
//
// CUE's close() has no special meaning in an expectation.
//
// Label cases for filtering with @label(slow, env:prod): after a case, inside
// it (or inside a struct it is built from), or at the top of the file for all
// its cases. Every case is also labelled with its kind (component, trait,
// policy, application-policy, addon, workflow-step, source) and what it does
// (render, status, exec), and with upgrade when marked @upgrade.
// @pending(reason), in the same places, parks a case: it loads but does not
// run.
//
// Cases run against the definition as written. @upgrade(reason="...") marks a
// known dependency on KubeVela's CUE upgrader: the case is retried with the
// controller's default rewrites, plus any named, e.g.
// @upgrade(generic-default-guard, reason="..."), and reported as upgraded
// rather than failed. It fails once it passes as written, so the marker goes
// with the fix.
//
// Hooks build the platform step and source cases run on. Mark a field of
// provider calls, such as kube.#Apply or test.#Seed, with @before() or
// @after() to run it once around the file's cases, or @beforeEach() or
// @afterEach() to run it around each. Mark a top-level field, hidden or not:
// it may hold one call or structs of them at any depth, which all run, in
// order, so a call can use an earlier one's $returns. A mark on a nested
// field is an error.
//
// Calls run for real, unmocked, and are not among a case's calls. An object
// a call names without a namespace, in a hook or a check, is in the scope's:
// the case's around a case, default around the file. As in
// Ginkgo, a failing @before fails every case, a failing @beforeEach fails its
// case without running the step, after hooks run regardless, and when no case
// runs, no hook does.
//
// What hooks create is deleted, newest first, when their scope ends, pass or
// fail: after the case for @beforeEach and @afterEach, after the file for
// @before and @after. So is what a case's own resources: creates, and what its
// step creates, after the case. A call creates the objects its value names,
// or, for config.#CreateConfig, the Secret holding the Config; anything else
// a provider makes, such as the objects a Config's template outputs, is for
// an @after to delete. Objects that already existed are left, as are
// namespaces, since envtest cannot finish deleting one: undoing a change to
// an object that already existed is what an @afterEach is for.
package test

// #ComponentRender renders a component definition.
#ComponentRender: {
	_#case
	$test: "component-render"
	traits?: [..._#trait]
	expect: {
		_#renderExpect
		// traits holds each attached trait's outputs, by trait type.
		traits?: [string]: outputs?: [string]: {...}
	}
}

// #TraitRender renders a trait definition against a workload it patches.
// The workload is a component of type context.componentType, which defaults
// to "cuetest-workload".
#TraitRender: {
	_#case
	$test: "trait-render"
	// workload is the object the trait patches.
	workload: {...}
	expect: _#renderExpect
}

// #ComponentStatus renders a component, then evaluates its health and status
// against the rendered objects as the cluster reports them.
#ComponentStatus: {
	_#case
	$test: "component-status"
	traits?: [..._#trait]
	// observed is merged over the rendered objects, as the cluster would
	// report them: typically `status`, plus fields the API server defaults.
	// As on a real API server, a built-in kind keeps only the fields its type
	// has, so `status` on a ConfigMap is dropped.
	observed?: {
		output?: {...}
		outputs?: [string]: {...}
		// traits holds each attached trait's outputs as observed, by type.
		traits?: [string]: outputs?: [string]: {...}
	}
	expect: {
		_#statusExpect
		// traits holds each attached trait's health, message and details, by type.
		traits?: [string]: {
			healthy?: bool
			message?: _
			details?: [string]: _
		}
	}
}

// #TraitStatus renders a trait, then evaluates its health and status against
// its own outputs as the cluster reports them. A trait's status never sees
// the workload.
#TraitStatus: {
	_#case
	$test: "trait-status"
	workload: {...}
	observed?: outputs?: [string]: {...}
	expect: _#statusExpect
}

// #AddonRender renders an addon as `vela addon enable` would install it: its
// Application, with its components keyed by name, the objects it installs
// beside the Application, and the definitions, config templates, schemas and
// views it ships. It stops at the Application: rendering its components is
// for the definitions' own tests, which a definition the addon ships can have
// beside it. Addon templates compile without providers, so an addon case
// takes no mocks. The hub it renders against has registered every cluster
// parameter.clusters names, so an addon without a template that deploys to
// runtime clusters gets the topology policy enabling it on those would add.
#AddonRender: {
	$test: "addon-render"
	// addon is the addon's directory, relative to the test file.
	addon: string
	parameter?: {...}
	mocks?: [string]: [string]: _
	expect: {
		application?: {...}
		components?: [string]: {...}
		auxiliaries?: [...{...}]
		// definitions are keyed by kind, then name, as in
		// definitions: ComponentDefinition: "shop-web": {...}.
		definitions?: [string]: [string]: {...}
		// configTemplates, schemas and views are keyed by name.
		configTemplates?: [string]: {...}
		schemas?: [string]: {...}
		views?: [string]: {...}
		error?: _
	}
}

// #WorkflowStepExec runs a workflow step as the workflow engine does,
// against a local API server with no controllers. Providers run for real
// unless mocked: kube and the rest act on that cluster, while calls that
// reach outside it (http, email, metrics, helm, registry, addon) or need an
// Application (oam, multicluster deploy) must be mocked. Each case gets a
// namespace of its own unless its context names one.
#WorkflowStepExec: {
	_#case
	$test: "workflowstep-exec"
	// resources are objects created before the step runs, with any status
	// set as a controller would have: nothing reconciles, so nothing becomes
	// ready unless a case says it is.
	resources?: [...{...}]
	// crds are CRD files or directories to install, relative to the test file.
	crds?: [...string]
	expect: {
		// phase is the step's phase: succeeded, failed, running (waiting),
		// suspending, skipped or pending.
		phase?:   string
		message?: _
		reason?:  _
		// calls holds every provider call, real or mocked, with $params and
		// what it returned, by import path then definition.
		calls?: [string]: [string]: _
		// resources are objects to read back after the step, each named by
		// apiVersion, kind and metadata.name (namespace defaults to the case's).
		// One that does not exist reads as empty. It is shorthand for a check
		// that calls kube.#Read and expects the object in returns.value.
		resources?: [...{...}]
		// checks run after the step, before @afterEach, each calling a provider
		// function for real, unmocked, and matching what it returns:
		// {call: kube.#Read & {...}, returns: value: data: a: "1"}. They run in
		// order, so a check can use an earlier one's call.$returns. returns is
		// matched like the rest of expect, attributes included. kube.#Read of
		// a missing object returns err rather than failing, with value still
		// the object asked for, so returns: err: =~"not found" expects one
		// absent.
		checks?: [string]: {
			call: {...}
			returns?: _
		}
		error?: _
	}
}

// #PolicyRender renders a workload-bearing policy definition, one with a
// template that is not Application-scoped, as the controller does before
// dispatching what it renders beside the Application's components: its
// output and outputs, named after the policy and in the Application's
// namespace unless they say otherwise, labelled as the Application's.
#PolicyRender: {
	_#case
	$test: "policy-render"
	// artifacts are the Application's rendered components, as the policy
	// reads them in context.artifacts: by component name, its workload and
	// each trait's objects by trait type then output.
	artifacts?: [string]: {
		workload?: {...}
		traits?: [string]: [string]: {...}
	}
	expect: _#renderExpect
}

// #ApplicationPolicyRender renders an Application-scoped policy definition as
// the controller does before it parses the Application: one policy, given the
// Application's spec, and its output applied as the next policy would see
// it. Labels and annotations merge into the Application's; components,
// workflow and policies replace its own where given. Several policies'
// ordering, and global policies, are the controller's concern.
#ApplicationPolicyRender: {
	_#case
	$test: "application-policy-render"
	// spec is the Application's spec as the policy receives it, read as
	// context.appComponents, context.appWorkflow and context.appPolicies.
	spec?: {
		components?: [...{...}]
		workflow?: {...}
		policies?: [...{...}]
	}
	expect: {
		// enabled is the template's enabled (or config.enabled), default true.
		enabled?: bool
		// output is what the template emitted: components, workflow, policies,
		// labels, annotations and ctx.
		output?: {...}
		// application is the Application with the output applied: metadata
		// (name, namespace, labels, annotations) and spec.
		application?: {...}
		calls?: [string]: [string]: _
		error?: _
	}
}

// #SourceExec resolves a source definition through the controller's own
// source engine, as an Application reading it would, against a local API
// server with no controllers. vela/kube and vela/velaconfig read that cluster
// for real unless mocked; vela/http and vela/registry reach outside it, so
// must be mocked. context holds the fields the consumer lets a source read,
// which are the fields its cache key can vary with: context.name is the
// binding, and the namespace defaults to one of the case's own. Nothing is
// cached, so every case fetches: expect.storage checks how the value would be
// cached, and how the cache behaves across reconciles is the controller's
// concern.
#SourceExec: {
	_#case
	$test: "source-exec"
	// consumer is the surface reading the source, which decides the context
	// it can read; it must be one the definition's consumableFrom allows.
	consumer: *"component" | "trait" | "workflowstep" | "policy-rendered"
	// resources are objects created before the source resolves, with any
	// status set as a controller would have.
	resources?: [...{...}]
	// crds are CRD files or directories to install, relative to the test file.
	crds?: [...string]
	expect: {
		// output is the source's resolved output, after the engine has checked
		// it against the definition's schema.
		output?: {...}
		// calls holds every provider call, real or mocked, by import path then
		// definition.
		calls?: [string]: [string]: _
		// storage is how the resolved value is cached, as the resolver
		// computes it: ttl (compared as a duration, so "90s" is "1m30s"),
		// onStaleFailure, keyInputs, the context fields the key varies with
		// and so which Applications share an entry, and the key itself.
		storage?: {
			ttl?:            string
			onStaleFailure?: string
			keyInputs?: [...string]
			key?: _
		}
		error?: _
	}
}

// _#trait attaches a trait to the component under test. Traits are
// evaluated in order, each patching the workload the one before it left.
_#trait: {
	// definition is the path of the trait's .cue file, as for the component.
	definition: string
	parameter?: {...}
}

_#case: {
	// definition is the path of the definition's .cue file, relative to the
	// test file; ".cue" may be omitted, so "webservice" is webservice.cue beside it.
	definition: string
	// context sets fields of the `context` the definition sees. The fields
	// allowed, and their types, are those the context registry
	// (pkg/definition/propexpr/context.cue) offers the definition's surface,
	// less those the case determines itself, such as a component's
	// componentType.
	context?: {...}
	parameter?: {...}
	// mocks answer the calls templates make to side-effecting providers, by
	// import path then definition, e.g. "vela/kube": "#Get". Each is one mock
	// or a list, tried in order: {$params?, $returns?}. A mock without $params
	// answers every call; with it, calls whose $params include those fields.
	// A legacy vela/op function takes its parameters and gives its results
	// beside them, so its mock matches and fills those fields. What an
	// unmatched call does depends on the test: a render fails it rather than
	// reach a cluster or the network, while #WorkflowStepExec and #SourceExec
	// run it for real against their local cluster unless it reaches outside
	// or needs an Application. vela/base64 and vela/cue, and in a step
	// vela/builtin, cannot be mocked and always run for real.
	mocks?: [string]: [string]: _
}

_#renderExpect: {
	output?: {...}
	outputs?: [string]: {...}
	// context is the `context` templates see once the component and every
	// attached trait have rendered: context.output after the traits' patches,
	// context.outputs with every output by name (a later trait's replacing an
	// earlier one of the same name), context.name, context.appName and the
	// rest. Unlike output and outputs, it is before the controller's labels.
	context?: {...}
	// calls holds the provider calls the render made, by import path then
	// definition, each a list in the order they ran of {$params, $returns?}.
	// A list fixes which calls and their order; add `...` to allow more, or
	// @contains() for any order, and `"#Apply"?: _|_` asserts none were made.
	calls?: [string]: [string]: _
	// error matches the render error; a case expecting one expects nothing else.
	error?: _
}

_#statusExpect: {
	healthy?: bool
	message?: _
	details?: [string]: _
	// error matches a render or status evaluation error.
	error?: _
}

// An expected error is either a string constraint matched against the whole
// message, such as =~"replicas", or a struct of its messages by category:
//
//	message:   the whole message
//	user:      what the definition raised through `errs`
//	parameter: parameter validation errors
//	template:  the rest of the template's errors, and any other render failure
//	status:    errors evaluating health, message or details
//	schema:    how a source's output does not fit its schema
//
// A category is present only when it has messages, so `template?: _|_`
// asserts the template was fine. Any other field fails the load.

// #Seed creates objects on the test cluster in a hook, then sets any status
// they carry, as a controller would have: nothing reconciles, so nothing
// becomes ready unless a hook says it is. kube.#Apply cannot do that, since
// the API server drops status on a create or apply. A namespace an object
// names that does not exist is created. Unlike kube.#Apply, an object that
// already exists is an error.
#Seed: {
	#do:       "seed"
	#provider: "test"
	$params: objects: [...{...}]
}

// #InstallCRDs installs CRD files or directories, relative to the test
// file, in a hook.
#InstallCRDs: {
	#do:       "install-crds"
	#provider: "test"
	$params: paths: [...string]
}
