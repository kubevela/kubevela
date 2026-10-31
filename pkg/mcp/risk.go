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
	securityPath = regexp.MustCompile(`(?i)(securityContext|serviceAccountName|automountServiceAccountToken|hostNetwork|hostPID|hostIPC|privileged|capabilities|allowPrivilegeEscalation|runAsUser|runAsRoot|hostPath|imagePullSecrets|tls|ingressClassName|externalIPs|loadBalancerSourceRanges)`)
	statefulPath = regexp.MustCompile(`(?i)(volumeClaimTemplates|persistentVolumeClaim|storageClassName|storage|serviceName|podManagementPolicy)`)
	configPath   = regexp.MustCompile(`(?i)(\.env\b|\.env\[|envFrom|configMap|\.args|\.command|volumes|volumeMounts|\.data\.|annotations|labels|livenessProbe|readinessProbe|startupProbe|ports|resources\.(requests|limits))`)
	scalingPath  = regexp.MustCompile(`(?i)(\.replicas$|minReplicas|maxReplicas|targetCPUUtilizationPercentage|metrics\[)`)
	replacePath  = regexp.MustCompile(`(?i)(^spec\.selector|^spec\.clusterIP$|^spec\.template\.metadata\.labels|^spec\.completions$|^spec\.parallelism$)`)
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
			case !cs.NewApplication:
				fire("addition", RiskMedium, id+" would be created in an existing Application")
			}
			continue
		}
		if securityKinds[rc.Kind] {
			fire("security-surface", RiskHigh, id+" is a security-relevant object and changed")
		}
		if statefulKinds[rc.Kind] {
			fire("stateful", RiskHigh, id+" is a stateful object and changed")
		}
		if rc.FieldsTruncated {
			fire("unclassified", RiskMedium, id+" has more field changes than were examined")
		}
		for _, f := range rc.Fields {
			classifyField(fire, id, rc.Kind, f)
		}
	}
	return r
}

// classifyField applies the path rules in order of severity and fires the
// first that matches, so one field yields one rule.
func classifyField(fire func(rule, class, evidence string), id, kind string, f FieldChange) {
	p := f.Path
	at := fmt.Sprintf("%s %s", id, p)
	switch {
	case replacePath.MatchString(p):
		fire("replace", RiskHigh, at+" is immutable; the object would be replaced")
	case securityPath.MatchString(p):
		fire("security-surface", RiskHigh, at)
	case kind == "Service" && p == "spec.type" && serviceTypeExposing[fmt.Sprint(f.To)]:
		fire("security-surface", RiskHigh, at+" exposes the Service outside the cluster")
	case statefulPath.MatchString(p):
		fire("stateful", RiskHigh, at)
	case imagePath.MatchString(p):
		if sameRepository(fmt.Sprint(f.From), fmt.Sprint(f.To)) {
			fire("image-tag", RiskLow, at)
		} else {
			fire("image-repository", RiskHigh, at+" changes the image repository, not only the tag")
		}
	case scalingPath.MatchString(p):
		fire("scaling", RiskLow, at)
	case configPath.MatchString(p):
		fire("config", RiskMedium, at)
	default:
		fire("unclassified", RiskMedium, at)
	}
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
