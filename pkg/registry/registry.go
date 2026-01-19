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

// Package registry provides a minimal interface-based provider registry for
// breaking import cycles in the codebase.
//
// This is a fallback mechanism for situations where import cycles block development.
//
// See README.md in this directory for detailed rationale, usage guidelines, and examples.
package registry

import (
	"fmt"
	"reflect"
	"sync"
)

var globalRegistry = &registry{
	providers: make(map[reflect.Type]interface{}),
}

type registry struct {
	mu        sync.RWMutex
	providers map[reflect.Type]interface{}
}

// RegisterAs registers an implementation for an interface type T.
//
// The type parameter T must be an interface type, and impl must be a non-nil
// implementation of that interface. If T is not an interface or impl is nil,
// this function panics with a descriptive error message.
//
// If a provider is registered multiple times, the last registration wins.
// This is intentional to allow test code to override providers.
//
// Example:
//
//	type MyProvider interface {
//	    DoSomething() error
//	}
//
//	type myImpl struct{}
//	func (m *myImpl) DoSomething() error { return nil }
//
//	registry.RegisterAs[MyProvider](&myImpl{})
func RegisterAs[T any](impl T) {
	interfaceType := reflect.TypeOf((*T)(nil)).Elem()

	// Validate T is an interface
	if interfaceType.Kind() != reflect.Interface {
		panic(fmt.Sprintf("registry.RegisterAs: type parameter T must be an interface type, got %s", interfaceType))
	}

	// Validate impl is not nil
	implValue := reflect.ValueOf(impl)
	if !implValue.IsValid() {
		panic(fmt.Sprintf("registry.RegisterAs: cannot register nil implementation for interface %s", interfaceType))
	}

	// Check for nil only on types that support IsNil() to avoid panics
	// IsNil() only works on: chan, func, interface, map, pointer, slice
	//nolint:exhaustive // Default case intentionally handles all other reflect.Kind values
	switch implValue.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if implValue.IsNil() {
			panic(fmt.Sprintf("registry.RegisterAs: cannot register nil implementation for interface %s", interfaceType))
		}
	default:
		// Other types (Invalid, Bool, Int*, Uint*, Float*, Complex*, Array, String, Struct, UnsafePointer)
		// either cannot be nil or are already caught by the IsValid() check above
	}

	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.providers[interfaceType] = impl
}

// Get retrieves the registered implementation for interface type T.
//
// Returns (implementation, true) if a provider is registered for type T,
// or (zero value, false) if no provider is registered.
//
// This function is thread-safe and optimized for concurrent access.
//
// Example:
//
//	if provider, ok := registry.Get[MyProvider](); ok {
//	    provider.DoSomething()
//	} else {
//	    // handle missing provider
//	}
func Get[T any]() (T, bool) {
	interfaceType := reflect.TypeOf((*T)(nil)).Elem()

	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	if impl, ok := globalRegistry.providers[interfaceType]; ok {
		return impl.(T), true
	}

	var zero T
	return zero, false
}

// RegistrySnapshot represents a saved state of the registry.
// Use with Restore() to save and restore registry state in tests.
type RegistrySnapshot struct {
	providers map[reflect.Type]interface{}
}

// Snapshot creates a copy of the current registry state.
//
// This is useful in tests to save the registry state (including bootstrap providers),
// make temporary changes, and then restore the original state.
//
// Example in tests:
//
//	func TestSomething(t *testing.T) {
//	    snapshot := registry.Snapshot()
//	    defer registry.Restore(snapshot)
//
//	    registry.RegisterAs[MyProvider](mockImpl)
//	    // ... test code that uses mockImpl
//
//	} // Restore() brings back all original providers including bootstrap ones
func Snapshot() RegistrySnapshot {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	// Create a deep copy of the providers map
	providersCopy := make(map[reflect.Type]interface{}, len(globalRegistry.providers))
	for k, v := range globalRegistry.providers {
		providersCopy[k] = v
	}

	return RegistrySnapshot{providers: providersCopy}
}

// Restore replaces the current registry state with a saved snapshot.
//
// This function is intended for testing only. It restores the registry
// to the exact state it was in when Snapshot() was called.
//
// Example in tests:
//
//	func TestSomething(t *testing.T) {
//	    snapshot := registry.Snapshot()
//	    defer registry.Restore(snapshot) // Restore original state
//
//	    registry.RegisterAs[MyProvider](mockImpl)
//	    // ... test code
//	}
func Restore(snapshot RegistrySnapshot) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	// Create a copy to preserve snapshot immutability
	providersCopy := make(map[reflect.Type]interface{}, len(snapshot.providers))
	for k, v := range snapshot.providers {
		providersCopy[k] = v
	}
	globalRegistry.providers = providersCopy
}
