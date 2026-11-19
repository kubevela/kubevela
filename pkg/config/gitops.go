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

package config

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The markers GitOps and release tools leave on what they apply.
const (
	argoCDTrackingIDAnnotation = "argocd.argoproj.io/tracking-id"
	fluxKustomizationLabel     = "kustomize.toolkit.fluxcd.io/name"
	fluxHelmReleaseLabel       = "helm.toolkit.fluxcd.io/name"
	instanceLabel              = "app.kubernetes.io/instance"
	managedByLabel             = "app.kubernetes.io/managed-by"
)

// EventMigratedGitOpsManaged is recorded on the CR that adopted a legacy object a GitOps or
// release tool manages.
const EventMigratedGitOpsManaged = "MigratedGitOpsManaged"

// GitOpsManager names the tool whose marker obj carries and the marker itself, or two empty
// strings. The vela CLI never sets these markers on a legacy config object, so a match means a
// person or a tool put it there, and a tool that applies an object applies it again: the owner
// should keep the new CR where the legacy object is kept. Helm itself stamps managed-by=Helm on
// what it installs; the instance label comes from a chart's templates. app.kubernetes.io/instance
// without it is what Argo CD's label tracking sets, and also what many charts and hand-written manifests
// set, so it is reported as possible, not as Argo CD.
func GitOpsManager(obj metav1.Object) (tool, marker string) {
	annotations, labels := obj.GetAnnotations(), obj.GetLabels()
	switch {
	case annotations[argoCDTrackingIDAnnotation] != "":
		return "Argo CD", "annotation " + argoCDTrackingIDAnnotation
	case labels[fluxKustomizationLabel] != "":
		return "Flux", "label " + fluxKustomizationLabel
	case labels[fluxHelmReleaseLabel] != "":
		return "Flux", "label " + fluxHelmReleaseLabel
	case labels[managedByLabel] == "Helm":
		return "Helm", fmt.Sprintf("label %s=Helm", managedByLabel)
	case labels[instanceLabel] != "":
		return "Argo CD (label tracking) or another tool", "label " + instanceLabel
	}
	return "", ""
}

// gitOpsNote is the line a migration adds, to its report row and to a MigratedGitOpsManaged
// Event on the new CR, when the legacy object it adopted carries a tool's marker; "" when it
// carries none. kind is the legacy object's kind, crKind the CR's.
func gitOpsNote(legacy metav1.Object, kind, crKind string) string {
	tool, marker := GitOpsManager(legacy)
	switch tool {
	case "":
		return ""
	case "Argo CD (label tracking) or another tool":
		return fmt.Sprintf("the legacy %s may be managed by %s (%s): if so, keep this %s where it is kept instead, so the next sync does not reapply the legacy object", kind, tool, marker, crKind)
	}
	return fmt.Sprintf("the legacy %s is managed by %s (%s): keep this %s where it is kept instead, so the next sync does not reapply the legacy object", kind, tool, marker, crKind)
}
