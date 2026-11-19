/*
Copyright 2025 The KubeVela Authors.

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

package defkit

// ConditionalParamBlock represents a set of conditional parameter branches.
// Each branch has a guard condition and emits different parameters when active.
//
// Example:
//
//	ConditionalParams(
//	    WhenParam(existingResources.Eq(false)).Params(
//	        defkit.Bool("forceDestroy").Default(false),
//	    ),
//	    WhenParam(existingResources.Eq(true)).Params(
//	        defkit.Bool("forceDestroy").Optional(),
//	    ),
//	)
//
// Emits:
//
//	if existingResources == false {
//	    forceDestroy: *false | bool
//	}
//	if existingResources == true {
//	    forceDestroy?: bool
//	}
type ConditionalParamBlock struct {
	branches []*ConditionalBranch
}

// ConditionalParams creates a new conditional parameter block from branches.
func ConditionalParams(branches ...*ConditionalBranch) *ConditionalParamBlock {
	return &ConditionalParamBlock{branches: branches}
}

// Branches returns all conditional branches.
func (b *ConditionalParamBlock) Branches() []*ConditionalBranch {
	return b.branches
}

// ConditionalBranch represents a single branch in a conditional parameter block.
// It contains a guard condition, parameters to emit when active, and optional validators.
type ConditionalBranch struct {
	condition  Condition
	params     []Param
	validators []*Validator
}

// WhenParam creates a new conditional branch with the given guard condition.
func WhenParam(cond Condition) *ConditionalBranch {
	return &ConditionalBranch{condition: cond}
}

// Params sets the parameters emitted when this branch's condition is true.
func (b *ConditionalBranch) Params(params ...Param) *ConditionalBranch {
	b.params = append(b.params, params...)
	return b
}

// Validators adds validators emitted inside this branch's conditional block.
func (b *ConditionalBranch) Validators(validators ...*Validator) *ConditionalBranch {
	b.validators = append(b.validators, validators...)
	return b
}

// Condition returns the branch's guard condition.
func (b *ConditionalBranch) Condition() Condition { return b.condition }

// GetParams returns the branch's parameters.
func (b *ConditionalBranch) GetParams() []Param { return b.params }

// GetValidators returns the branch's validators.
func (b *ConditionalBranch) GetValidators() []*Validator { return b.validators }
