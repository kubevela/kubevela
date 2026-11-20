/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package cli

import (
	"testing"

	"github.com/oam-dev/kubevela/pkg/cue/upgrade"
)

func TestEnableAllUpgradePasses(t *testing.T) {
	origList := upgrade.EnableListConcatUpgrade
	origErr := upgrade.EnableErrorFieldLabelUpgrade
	origBool := upgrade.EnableBoolDefaultGuardUpgrade
	origGeneric := upgrade.EnableGenericDefaultGuardUpgrade
	origKeep := upgrade.EnableKeepValidatorsSingletonUpgrade
	origEval := upgrade.EnableEvalv3SelfRefGuardUpgrade
	t.Cleanup(func() {
		upgrade.EnableListConcatUpgrade = origList
		upgrade.EnableErrorFieldLabelUpgrade = origErr
		upgrade.EnableBoolDefaultGuardUpgrade = origBool
		upgrade.EnableGenericDefaultGuardUpgrade = origGeneric
		upgrade.EnableKeepValidatorsSingletonUpgrade = origKeep
		upgrade.EnableEvalv3SelfRefGuardUpgrade = origEval
	})

	upgrade.EnableListConcatUpgrade = false
	upgrade.EnableErrorFieldLabelUpgrade = false
	upgrade.EnableBoolDefaultGuardUpgrade = false
	upgrade.EnableGenericDefaultGuardUpgrade = false
	upgrade.EnableKeepValidatorsSingletonUpgrade = false
	upgrade.EnableEvalv3SelfRefGuardUpgrade = false

	restore := enableAllUpgradePasses(true)

	if !upgrade.EnableListConcatUpgrade ||
		!upgrade.EnableErrorFieldLabelUpgrade ||
		!upgrade.EnableBoolDefaultGuardUpgrade ||
		!upgrade.EnableGenericDefaultGuardUpgrade ||
		!upgrade.EnableKeepValidatorsSingletonUpgrade ||
		!upgrade.EnableEvalv3SelfRefGuardUpgrade {
		t.Fatalf("expected all per-pass upgrade toggles to be true when --enable-all is set")
	}

	restore()

	if upgrade.EnableListConcatUpgrade != false ||
		upgrade.EnableErrorFieldLabelUpgrade != false ||
		upgrade.EnableBoolDefaultGuardUpgrade != false ||
		upgrade.EnableGenericDefaultGuardUpgrade != false ||
		upgrade.EnableKeepValidatorsSingletonUpgrade != false ||
		upgrade.EnableEvalv3SelfRefGuardUpgrade != false {
		t.Fatalf("expected per-pass upgrade toggles to be restored after command execution")
	}
}
