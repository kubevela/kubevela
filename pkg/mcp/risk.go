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

package mcp

import (
	"fmt"
	"regexp"
	"strings"
)

// Risk classes, ordered.
const (
	RiskLow    = "LOW"
	RiskMedium = "MEDIUM"
	RiskHigh   = "HIGH"
)

var riskRank = map[string]int{RiskLow: 0, RiskMedium: 1, RiskHigh: 2}

// RiskRule is one rule that fired, with what it fired on.
type RiskRule struct {
	Rule     string `json:"rule" jsonschema:"Name of the rule that fired, such as image-tag, scaling, deletion or placement-changed."`
	Class    string `json:"class" jsonschema:"Class this rule assigns: LOW, MEDIUM or HIGH."`
	Evidence string `json:"evidence" jsonschema:"The object and field the rule fired on."`
}

// Risk is the classifier's verdict: the maximum class over every rule that
// fired, with all of them listed so a reader can see why.
type Risk struct {
	Class string     `json:"class" jsonschema:"Overall class, the highest of every rule that fired: LOW, MEDIUM or HIGH."`
	Rules []RiskRule `json:"rules" jsonschema:"Every rule that fired, so the class can be explained."`
}

var (
	securityPath = regexp.MustCompile(`(?i)(securityContext|serviceAccountName|automountServiceAccountToken|hostNetwork|hostPID|hostIPC|privileged|capabilities|allowPrivilegeEscalation|runAsUser|runAsRoot|hostPath|hostUsers|shareProcessNamespace|hostPort|imagePullSecrets|tls|ingressClassName|externalIPs|loadBalancerSourceRanges)`)
	statefulPath = regexp.MustCompile(`(?i)(volumeClaimTemplates|persistentVolumeClaim|storageClassName|resources\.requests\.storage$|serviceName|podManagementPolicy)`)
	configPath   = regexp.MustCompile(`(?i)(\.env\b|\.env\[|envFrom|configMap|\.args|\.command|volumes|volumeMounts|(^|\.)data\.|annotations|labels|livenessProbe|readinessProbe|startupProbe|ports|resources\.(requests|limits))`)
	scalingPath  = regexp.MustCompile(`(?i)(\.replicas$|minReplicas|maxReplicas|targetCPUUtilizationPercentage|metrics\[)`)
	imagePath    = regexp.MustCompile(`(?i)containers\[\d+\]\.image$`)
	// serviceTypeExposing matches a Service type change that opens the
	// workload to traffic from outside the cluster.
	serviceTypeExposing = map[string]bool{"NodePort": true, "LoadBalancer": true}
)

var (
	securityKinds = map[string]bool{"Role": true, "ClusterRole": true, "RoleBinding": true, "ClusterRoleBinding": true, "NetworkPolicy": true, "Ingress": true, "Gateway": true, "HTTPRoute": true, "ServiceAccount": true, "PodSecurityPolicy": true, "Secret": true, "Certificate": true, "Issuer": true, "ClusterIssuer": true}
	statefulKinds = map[string]bool{"StatefulSet": true, "PersistentVolumeClaim": true, "PersistentVolume": true, "StorageClass": true}
)

// classify is a pure function from a change set to a risk. It implements
// the rules the KEP lists: image tag only, scaling and a new Application
// are LOW; config, env and additions to an existing Application are
// MEDIUM; any deletion, replace, placement change, security-surface
// change, stateful change or removed gate is HIGH, including in a new
// Application. A change no rule recognises is MEDIUM, never LOW.
func classify(cs ChangeSet) Risk {
	r := Risk{Class: RiskLow, Rules: []RiskRule{}}
	fire := func(rule, class, evidence string) {
		r.Rules = append(r.Rules, RiskRule{Rule: rule, Class: class, Evidence: evidence})
		if riskRank[class] > riskRank[r.Class] {
			r.Class = class
		}
	}
	if cs.NewApplication {
		fire("new-application", RiskLow, "no revision of this Application exists; every resource is new")
	}
	if cs.GateRemoved {
		fire("gate-removed", RiskHigh, "the suspend step before the first deploy step is gone")
	}
	if cs.PlacementChanged {
		fire("placement-changed", RiskHigh, "topology policy or namespace differs from the running revision")
	}
	for _, name := range cs.ComponentsRemoved {
		fire("component-removed", RiskHigh, fmt.Sprintf("component %q is no longer declared", name))
	}
	if !cs.NewApplication {
		for _, name := range cs.ComponentsAdded {
			fire("component-added", RiskMedium, fmt.Sprintf("component %q is new to an existing Application", name))
		}
		for _, name := range cs.PoliciesChanged {
			fire("policy-changed", RiskMedium, fmt.Sprintf("policy %q changed", name))
		}
		if cs.WorkflowChanged && !cs.GateRemoved {
			fire("workflow-changed", RiskMedium, "the declared workflow differs from the running revision")
		}
		if cs.SourcesChanged {
			fire("sources-changed", RiskMedium, "the Application's sources differ from the running revision")
		}
		rendersChanged := map[string]bool{}
		for _, rc := range cs.Resources {
			rendersChanged[rc.Component] = true
		}
		for _, name := range cs.ComponentsChanged {
			if !rendersChanged[name] {
				fire("component-spec", RiskMedium, fmt.Sprintf("component %q changed but renders the same objects; the change is in how it is deployed, not what", name))
			}
		}
	}
	for _, rc := range cs.Resources {
		id := fmt.Sprintf("%s %s", rc.Kind, rc.Name)
		if rc.Namespace != "" {
			id = fmt.Sprintf("%s %s/%s", rc.Kind, rc.Namespace, rc.Name)
		}
		switch rc.Action {
		case ActionRemove:
			fire("deletion", RiskHigh, id+" would be deleted")
			continue
		case ActionAdd:
			switch {
			case securityKinds[rc.Kind]:
				fire("security-surface", RiskHigh, id+" is a new security-relevant object")
			case statefulKinds[rc.Kind]:
				fire("stateful", RiskHigh, id+" is a new stateful object")
			default:
				classifyAdded(fire, id, rc)
				if !cs.NewApplication {
					fire("addition", RiskMedium, id+" would be created in an existing Application")
				}
			}
			continue
		}
		if securityKinds[rc.Kind] {
			fire("security-surface", RiskHigh, id+" is a security-relevant object and changed")
		}
		if statefulKinds[rc.Kind] {
			fire("stateful", RiskHigh, id+" is a stateful object and changed")
		}
		fields := rc.examined
		if fields == nil {
			// A change set built elsewhere carries only the listed fields.
			fields = rc.Fields
			if rc.FieldsTruncated {
				fire("unclassified", RiskMedium, id+" has more field changes than were examined")
			}
		}
		for _, f := range fields {
			classifyField(fire, id, rc.APIVersion, rc.Kind, f)
		}
	}
	return r
}

// classifyAdded applies the security rules a change to an existing object
// would meet to the leaves a new object sets, so a new privileged workload
// or exposed Service is HIGH like the same change to an existing one.
func classifyAdded(fire func(rule, class, evidence string), id string, rc ResourceChange) {
	for _, f := range rc.examined {
		at := fmt.Sprintf("%s %s", id, f.Path)
		switch {
		case payloadLeaf(rc.Kind, f.Path):
			// user data under a Secret or ConfigMap payload; the kind rules
			// above already judged the object
		case securityPath.MatchString(f.Path):
			fire("security-surface", RiskHigh, at+" is set on a new object")
		case exposesService(rc.Kind, f):
			fire("security-surface", RiskHigh, at+" exposes the new Service outside the cluster")
		}
	}
}

// exposesService reports a Service type that opens the workload to traffic
// from outside the cluster.
func exposesService(kind string, f FieldChange) bool {
	return kind == "Service" && f.Path == "spec.type" && serviceTypeExposing[fmt.Sprint(f.To)]
}

// immutable reports whether the API server rejects an in-place change of
// the field on this kind, so applying it replaces the object: a workload's
// selector, a Service's clusterIP, and a Job's completions, pod template,
// success policy and scheduling configuration (whose gang minCount alone
// may change).
func immutable(apiVersion, kind, p string) bool {
	group := ""
	if i := strings.Index(apiVersion, "/"); i >= 0 {
		group = apiVersion[:i]
	}
	under := func(prefix string) bool { return p == prefix || strings.HasPrefix(p, prefix+".") }
	switch {
	case group == "apps" && (kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" || kind == "ReplicaSet"):
		return under("spec.selector")
	case group == "batch" && kind == "Job":
		return under("spec.selector") || p == "spec.completions" || under("spec.template") || under("spec.successPolicy") ||
			(under("spec.scheduling") && p != "spec.scheduling.schedulingPolicy.gang.minCount")
	case group == "" && kind == "Service":
		return p == "spec.clusterIP"
	}
	return false
}

// classifyField applies the path rules in order of severity and fires the
// first that matches, so one field yields one rule.
func classifyField(fire func(rule, class, evidence string), id, apiVersion, kind string, f FieldChange) {
	p := f.Path
	at := fmt.Sprintf("%s %s", id, p)
	switch {
	case immutable(apiVersion, kind, p):
		fire("replace", RiskHigh, at+" is immutable; the object would be replaced")
	case payloadLeaf(kind, p):
		// A payload key is user data named by the user: data.tls.crt or
		// data.hostPort in a ConfigMap is configuration, not a Pod field.
		fire("config", RiskMedium, at)
	case securityPath.MatchString(p):
		fire("security-surface", RiskHigh, at)
	case exposesService(kind, f):
		fire("security-surface", RiskHigh, at+" exposes the Service outside the cluster")
	case statefulPath.MatchString(p):
		fire("stateful", RiskHigh, at)
	case imagePath.MatchString(p):
		from, to := fmt.Sprint(f.From), fmt.Sprint(f.To)
		switch {
		case !sameRepository(from, to):
			fire("image-repository", RiskHigh, at+" changes the image repository, not only the tag")
		case strings.Contains(from, "@") || strings.Contains(to, "@"):
			fire("image-digest", RiskMedium, at+" changes a pinned digest, which says nothing about what the new image is")
		default:
			fire("image-tag", RiskLow, at)
		}
	case scalingPath.MatchString(p):
		fire("scaling", RiskLow, at)
	case configPath.MatchString(p):
		fire("config", RiskMedium, at)
	default:
		fire("unclassified", RiskMedium, at)
	}
}

// payloadLeaf reports a path inside the payload of a Secret or ConfigMap, the
// fields payloadFields names for the kind. Their keys are chosen by the user,
// so the path rules written for Pod and Service fields do not apply to them.
func payloadLeaf(kind, path string) bool {
	for _, field := range payloadFields(map[string]any{"kind": kind}) {
		if path == field || strings.HasPrefix(path, field+".") || strings.HasPrefix(path, field+"[") {
			return true
		}
	}
	return false
}

// sameRepository reports whether two image references differ only in tag
// or digest.
func sameRepository(a, b string) bool {
	return imageRepository(a) == imageRepository(b)
}

func imageRepository(ref string) string {
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	slash := strings.LastIndex(ref, "/")
	colon := strings.LastIndex(ref, ":")
	if colon > slash {
		ref = ref[:colon]
	}
	return ref
}
