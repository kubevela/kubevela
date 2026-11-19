/*
 Copyright 2026. The KubeVela Authors.

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

package operation

import (
	"testing"

	"github.com/stretchr/testify/require"

	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
)

func TestSetupRejectsNegativeDefaultOperationTTL(t *testing.T) {
	// The validation is the very first thing Setup does, before it ever
	// touches the manager, so a nil mgr is safe here -- if this ever
	// changed to read mgr first, this test would panic instead of
	// passing, which is exactly the signal we'd want.
	err := Setup(nil, core.Args{DefaultOperationTTLSeconds: -1})
	require.ErrorContains(t, err, "must be >= 0")
}
