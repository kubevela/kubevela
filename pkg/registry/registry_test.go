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

package registry

import (
	"strings"
	"sync"
	"testing"
)

// Test interfaces
type SimpleService interface {
	GetValue() string
}

type ComplexService interface {
	Process(input string) (string, error)
	Validate(data interface{}) bool
}

type AnotherService interface {
	Execute() int
}

// Test implementations
type simpleImpl struct {
	value string
}

func (s *simpleImpl) GetValue() string {
	return s.value
}

type complexImpl struct {
	prefix string
}

func (c *complexImpl) Process(input string) (string, error) {
	return c.prefix + input, nil
}

func (c *complexImpl) Validate(data interface{}) bool {
	return data != nil
}

type anotherImpl struct {
	result int
}

func (a *anotherImpl) Execute() int {
	return a.result
}

// TestRegisterAndGet_BasicInterface tests basic registration and retrieval
func TestRegisterAndGet_BasicInterface(t *testing.T) {
	snapshot := Snapshot()
	defer Restore(snapshot)

	impl := &simpleImpl{value: "test-value"}
	RegisterAs[SimpleService](impl)

	retrieved, ok := Get[SimpleService]()
	if !ok {
		t.Fatal("Expected to find registered service, but got false")
	}

	if retrieved.GetValue() != "test-value" {
		t.Errorf("Expected value 'test-value', got '%s'", retrieved.GetValue())
	}
}

// TestRegisterAndGet_Override tests that last write wins
func TestRegisterAndGet_Override(t *testing.T) {
	snapshot := Snapshot()
	defer Restore(snapshot)

	// Register first implementation
	impl1 := &simpleImpl{value: "first"}
	RegisterAs[SimpleService](impl1)

	// Register second implementation (override)
	impl2 := &simpleImpl{value: "second"}
	RegisterAs[SimpleService](impl2)

	// Should get the second implementation
	retrieved, ok := Get[SimpleService]()
	if !ok {
		t.Fatal("Expected to find registered service, but got false")
	}

	if retrieved.GetValue() != "second" {
		t.Errorf("Expected value 'second' (last registered), got '%s'", retrieved.GetValue())
	}
}

// TestGet_NotFound tests retrieval of unregistered service
func TestGet_NotFound(t *testing.T) {
	snapshot := Snapshot()
	defer Restore(snapshot)

	// Don't register anything, try to get
	retrieved, ok := Get[AnotherService]()
	if ok {
		t.Fatal("Expected to not find service, but got true")
	}

	// Verify zero value is returned
	if retrieved != nil {
		t.Errorf("Expected nil (zero value) for interface, got %v", retrieved)
	}
}

// TestRegisterAs_PanicsOnNonInterface tests that concrete types are rejected
func TestRegisterAs_PanicsOnNonInterface(t *testing.T) {
	snapshot := Snapshot()
	defer Restore(snapshot)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected panic when registering concrete type, but didn't panic")
		}

		panicMsg := r.(string)
		if !strings.Contains(panicMsg, "must be an interface type") {
			t.Errorf("Expected panic message about interface type, got: %s", panicMsg)
		}
	}()

	// Try to register a concrete struct type (should panic)
	type ConcreteStruct struct {
		Value string
	}
	concrete := ConcreteStruct{Value: "test"}
	RegisterAs[ConcreteStruct](concrete)
}

// TestRegisterAs_PanicsOnNil tests that nil implementations are rejected
func TestRegisterAs_PanicsOnNil(t *testing.T) {
	snapshot := Snapshot()
	defer Restore(snapshot)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected panic when registering nil, but didn't panic")
		}

		panicMsg := r.(string)
		if !strings.Contains(panicMsg, "cannot register nil") {
			t.Errorf("Expected panic message about nil, got: %s", panicMsg)
		}
	}()

	// Try to register nil (should panic)
	var nilImpl SimpleService
	RegisterAs[SimpleService](nilImpl)
}

// TestConcurrentGet tests that concurrent Get operations are thread-safe
func TestConcurrentGet(t *testing.T) {
	snapshot := Snapshot()
	defer Restore(snapshot)

	// Register multiple services
	RegisterAs[SimpleService](&simpleImpl{value: "concurrent-test"})
	RegisterAs[ComplexService](&complexImpl{prefix: "prefix-"})
	RegisterAs[AnotherService](&anotherImpl{result: 42})

	const numGoroutines = 100
	const numIterations = 100

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*3)

	// Launch goroutines that read concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(3)

		// Test SimpleService Get
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				svc, ok := Get[SimpleService]()
				if !ok {
					errors <- nil // Signal error
					return
				}
				if svc.GetValue() != "concurrent-test" {
					errors <- nil
					return
				}
			}
		}()

		// Test ComplexService Get
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				svc, ok := Get[ComplexService]()
				if !ok {
					errors <- nil
					return
				}
				result, _ := svc.Process("test")
				if result != "prefix-test" {
					errors <- nil
					return
				}
			}
		}()

		// Test AnotherService Get
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				svc, ok := Get[AnotherService]()
				if !ok {
					errors <- nil
					return
				}
				if svc.Execute() != 42 {
					errors <- nil
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for range errors {
		errorCount++
	}
	if errorCount > 0 {
		t.Errorf("Concurrent Get operations had %d errors", errorCount)
	}
}

// TestConcurrentRegisterAndGet tests concurrent RegisterAs and Get operations
func TestConcurrentRegisterAndGet(t *testing.T) {
	snapshot := Snapshot()
	defer Restore(snapshot)

	const numGoroutines = 50

	var wg sync.WaitGroup

	// Goroutines that register and override
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Each goroutine registers its own value
			impl := &anotherImpl{result: id}
			RegisterAs[AnotherService](impl)
		}(i)
	}

	// Goroutines that read
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Just verify Get doesn't panic or deadlock
			_, _ = Get[AnotherService]()
		}()
	}

	wg.Wait()

	// Verify we can still get a valid service after concurrent access
	svc, ok := Get[AnotherService]()
	if !ok {
		t.Fatal("Expected to find service after concurrent operations")
	}

	// Result should be one of the registered values (0 to numGoroutines-1)
	result := svc.Execute()
	if result < 0 || result >= numGoroutines {
		t.Errorf("Expected result in range [0, %d), got %d", numGoroutines, result)
	}
}

// TestSnapshotAndRestore tests the snapshot/restore functionality
func TestSnapshotAndRestore(t *testing.T) {
	// Save initial state to restore after test
	initialState := Snapshot()
	defer Restore(initialState)

	// Register initial services
	RegisterAs[SimpleService](&simpleImpl{value: "original"})
	RegisterAs[ComplexService](&complexImpl{prefix: "original-"})

	// Take a snapshot
	snapshot := Snapshot()

	// Modify the registry
	RegisterAs[SimpleService](&simpleImpl{value: "modified"})
	RegisterAs[AnotherService](&anotherImpl{result: 42})

	// Verify modifications took effect
	svc, ok := Get[SimpleService]()
	if !ok || svc.GetValue() != "modified" {
		t.Fatal("Expected modified value")
	}
	if _, ok := Get[AnotherService](); !ok {
		t.Fatal("Expected AnotherService to be registered")
	}

	// Restore the snapshot
	Restore(snapshot)

	// Verify original state is restored
	svc, ok = Get[SimpleService]()
	if !ok || svc.GetValue() != "original" {
		t.Errorf("Expected 'original' after restore, got '%s'", svc.GetValue())
	}

	complex, ok := Get[ComplexService]()
	if !ok {
		t.Fatal("ComplexService should be restored")
	}
	result, _ := complex.Process("test")
	if result != "original-test" {
		t.Errorf("Expected 'original-test', got '%s'", result)
	}

	// Verify AnotherService is gone (wasn't in snapshot)
	if _, ok := Get[AnotherService](); ok {
		t.Error("AnotherService should not exist after restore")
	}

	// Test cleanup happens via defer Restore(initialState)
}

// Value receiver types for testing
type valueReceiverStruct struct {
	data string
}

type valueReceiverInterface interface {
	GetData() string
}

func (v valueReceiverStruct) GetData() string {
	return v.data
}

// TestRegisterAs_StructValueReceiver tests registration with struct value receivers
func TestRegisterAs_StructValueReceiver(t *testing.T) {
	snapshot := Snapshot()
	defer Restore(snapshot)

	// Register a struct value (not a pointer)
	val := valueReceiverStruct{data: "test"}
	RegisterAs[valueReceiverInterface](val)

	retrieved, ok := Get[valueReceiverInterface]()
	if !ok {
		t.Fatal("Expected to retrieve registered struct value")
	}
	if retrieved.GetData() != "test" {
		t.Errorf("Expected 'test', got '%s'", retrieved.GetData())
	}
}

// TestSnapshotImmutability verifies that snapshots remain immutable after Restore
func TestSnapshotImmutability(t *testing.T) {
	// Save initial state to restore after test
	initialState := Snapshot()
	defer Restore(initialState)

	// Create a snapshot with a specific state
	RegisterAs[SimpleService](&simpleImpl{value: "snapshot-value"})
	snapshot := Snapshot()

	// First restore
	Restore(snapshot)

	// Modify the registry after restore
	RegisterAs[SimpleService](&simpleImpl{value: "modified-value"})
	RegisterAs[AnotherService](&anotherImpl{result: 999})

	// Second restore - should restore to original snapshot state
	// This would fail if Restore() didn't copy the map
	Restore(snapshot)

	// Verify the snapshot's state is preserved
	svc, ok := Get[SimpleService]()
	if !ok {
		t.Fatal("SimpleService should exist after second restore")
	}
	if svc.GetValue() != "snapshot-value" {
		t.Errorf("Expected 'snapshot-value' after second restore, got '%s'. Snapshot was corrupted!", svc.GetValue())
	}

	// Verify AnotherService is not present (wasn't in snapshot)
	if _, ok := Get[AnotherService](); ok {
		t.Error("AnotherService should not exist after restore - snapshot was corrupted!")
	}
}

// TestSnapshotPreservesBootstrapServices demonstrates the recommended pattern
func TestSnapshotPreservesBootstrapServices(t *testing.T) {
	// Save initial state to restore after test
	initialState := Snapshot()
	defer Restore(initialState)

	// Simulate bootstrap registering a service
	RegisterAs[SimpleService](&simpleImpl{value: "bootstrap-service"})

	// Test code that uses Snapshot/Restore
	snapshot := Snapshot()
	defer Restore(snapshot)

	// Override with mock for testing
	RegisterAs[SimpleService](&simpleImpl{value: "mock-service"})

	// Test code uses mock
	svc, ok := Get[SimpleService]()
	if !ok || svc.GetValue() != "mock-service" {
		t.Fatal("Expected mock service during test")
	}

	// Manually restore to verify bootstrap service comes back
	Restore(snapshot)

	svc, ok = Get[SimpleService]()
	if !ok || svc.GetValue() != "bootstrap-service" {
		t.Error("Bootstrap service should be restored after Restore()")
	}

	// Test cleanup happens via outer defer Restore(initialState)
}
