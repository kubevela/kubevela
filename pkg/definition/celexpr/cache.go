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

package celexpr

import (
	"sync"

	"github.com/google/cel-go/cel"
	"k8s.io/utils/lru"
)

// programCacheSize bounds the number of distinct expressions kept compiled.
const programCacheSize = 2048

var (
	dynEnvOnce sync.Once
	dynEnvVal  *cel.Env
	dynEnvErr  error

	// Keyed on expression text alone, which is sound only because entries are
	// stored for the shared permissive environment and no other. See compiledFor.
	//
	// lru.Cache locks internally, so this needs no mutex.
	compiledCache = lru.New(programCacheSize)
)

// compiled is one expression's compilation: the AST References walks and the
// program Eval runs. Both are read-only once built.
type compiled struct {
	ast *cel.Ast
	prg cel.Program
}

// compiledFor returns expr compiled against env, reusing an earlier compilation
// where it can.
//
// Only the shared permissive environment is cached, and the identity check is
// what keeps that sound. A typed environment from EnvForContext declares each
// binding's real shape, so the same text compiles to a different result there;
// handing it a dyn-typed program would disable the target-type check that stops
// a string reaching an int parameter.
//
// Failures are not cached. They are rare, and an error's text should reflect the
// call that produced it.
func compiledFor(env *cel.Env, expr string) (*compiled, error) {
	shared, sharedErr := DynEnv()
	cacheable := sharedErr == nil && env == shared

	if cacheable {
		if hit, ok := compiledCache.Get(expr); ok {
			if c, ok := hit.(*compiled); ok {
				return c, nil
			}
		}
	}

	ast, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, iss.Err()
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, err
	}
	c := &compiled{ast: ast, prg: prg}

	if cacheable {
		compiledCache.Add(expr, c)
	}
	return c, nil
}
