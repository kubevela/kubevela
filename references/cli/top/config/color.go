/*
Copyright 2022 The KubeVela Authors.

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
	"github.com/gdamore/tcell/v2"
)

const (
	// InfoSectionColor system info component section text color
	InfoSectionColor = tcell.ColorRoyalBlue
	// InfoTextColor system info component text color
	InfoTextColor = tcell.ColorLightGray
	// LogoTextColor logo text color
	LogoTextColor = tcell.ColorRoyalBlue
	// CrumbsBackgroundColor crumbs background color
	CrumbsBackgroundColor = tcell.ColorRoyalBlue
	// ResourceTableTitleColor resource component title color
	ResourceTableTitleColor = tcell.ColorBlue
	// ResourceTableHeaderColor resource table header text color
	ResourceTableHeaderColor = tcell.ColorLightGray
	// ResourceTableBodyColor resource table body text color
	ResourceTableBodyColor = tcell.ColorBlue
	// ApplicationStartingAndRenderingPhaseColor application Starting and Rendering phase text color
	ApplicationStartingAndRenderingPhaseColor = "[blue::]"
	// ApplicationWorkflowSuspendingPhaseColor application WorkflowSuspending phase text color
	ApplicationWorkflowSuspendingPhaseColor = "[yellow::]"
	// ApplicationWorkflowTerminatedPhaseColor application WorkflowTerminated phase text color
	ApplicationWorkflowTerminatedPhaseColor = "[red::]"
	// ApplicationRunningPhaseColor application Running phase text color
	ApplicationRunningPhaseColor = "[green::]"
	// NamespaceActiveStatusColor is namespace active status text color
	NamespaceActiveStatusColor = "[green::]"
	// NamespaceTerminateStatusColor is namespace terminate status text color
	NamespaceTerminateStatusColor = "[red::]"
	// ObjectHealthyStatusColor is object Healthy status text color
	ObjectHealthyStatusColor = "[green::]"
	// ObjectUnhealthyStatusColor is object Unhealthy status text color
	ObjectUnhealthyStatusColor = "[red::]"
	// ObjectProgressingStatusColor is object Progressing status text color
	ObjectProgressingStatusColor = "[blue::]"
	// ObjectUnKnownStatusColor is object UnKnown status text color
	ObjectUnKnownStatusColor = "[gray::]"
	// PodPendingPhaseColor is pod pending phase text color
	PodPendingPhaseColor = "[yellow::]"
	// PodRunningPhaseColor is pod running phase text color
	PodRunningPhaseColor = "[green::]"
	// PodSucceededPhase is pod succeeded phase text color
	PodSucceededPhase = "[purple::]"
	// PodFailedPhase is pod failed phase text color
	PodFailedPhase = "[red::]"
)
