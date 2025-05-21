package main

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/holoplot/go-avahi"
	"ldddns.arnested.dk/internal/log"
)

// mockAvahiServer is a mock implementation of AvahiServer for testing.
type mockAvahiServer struct {
	EntryGroupNewFunc func() (*avahi.EntryGroup, error)
}

func (m *mockAvahiServer) EntryGroupNew() (*avahi.EntryGroup, error) {
	if m.EntryGroupNewFunc != nil {
		return m.EntryGroupNewFunc()
	}
	return nil, errors.New("EntryGroupNewFunc not implemented in mock")
}

// mockAvahiEntryGroup is a mock implementation of AvahiEntryGroup for testing.
type mockAvahiEntryGroup struct {
	avahi.EntryGroup // Embed to satisfy the interface if methods are added later

	IsEmptyFunc    func() (bool, error)
	CommitFunc     func() error
	ResetFunc      func() error
	AddServiceFunc func(iface int32, protocol int32, flags uint32, name string, stype string, domain string, host string, port uint16, txt [][]byte) error
	AddAddressFunc func(iface int32, protocol int32, flags uint32, name string, address string) error

	// Tracking calls
	commitCalled bool
	resetCalled  bool
}

func (m *mockAvahiEntryGroup) IsEmpty() (bool, error) {
	if m.IsEmptyFunc != nil {
		return m.IsEmptyFunc()
	}
	return false, errors.New("IsEmptyFunc not implemented in mock")
}

func (m *mockAvahiEntryGroup) Commit() error {
	m.commitCalled = true
	if m.CommitFunc != nil {
		return m.CommitFunc()
	}
	return errors.New("CommitFunc not implemented in mock")
}

func (m *mockAvahiEntryGroup) Reset() error {
	m.resetCalled = true
	if m.ResetFunc != nil {
		return m.ResetFunc()
	}
	return errors.New("ResetFunc not implemented in mock")
}

// Implement other avahi.EntryGroup methods if they are called by the code under test.
// For now, these are the ones directly used or potentially used by commit logic.

func TestEntryGroupsGet_Success(t *testing.T) {
	// Suppress logging during tests
	originalLogger := log.Logger
	log.Logger = func(priority log.Priority, format string, args ...interface{}) {}
	defer func() { log.Logger = originalLogger }()

	mockEntryGroup := &mockAvahiEntryGroup{}
	mockServer := &mockAvahiServer{
		EntryGroupNewFunc: func() (*avahi.EntryGroup, error) {
			// We need to return a real *avahi.EntryGroup, but with our mock's methods.
			// This is tricky because avahi.EntryGroup is a struct from an external package.
			// The typical way is to have the mock *be* the interface.
			// For go-avahi, EntryGroup is a concrete type, not an interface.
			// This means we need to be careful. The methods are on the concrete type.
			// The solution here is that our functions in entry_groups.go take *avahi.EntryGroup
			// So we can pass our mock directly if we make it satisfy the methods called.
			// However, the *avahi.Server returns a concrete *avahi.EntryGroup.
			// This means our mockAvahiServer must return an *avahi.EntryGroup.
			// The simplest way is to make mockAvahiEntryGroup an actual *avahi.EntryGroup
			// and override methods. But we can't directly override.
			//
			// The path of least resistance is to make our mockAvahiEntryGroup struct
			// implement the methods we expect to be called, and then cast it to
			// *avahi.EntryGroup for the return type of EntryGroupNewFunc.
			// This is unsafe if the underlying code tries to access uninitialized fields
			// of the real avahi.EntryGroup.
			//
			// A safer approach: the methods in entry_groups.go (IsEmpty, Commit)
			// are called on the `group` variable. If `group` is of type `*avahi.EntryGroup`,
			// then it must be a real one or one that has the same memory layout for called methods.
			//
			// Let's assume for now that we can make our mockEntryGroup behave like *avahi.EntryGroup
			// for the methods it implements. The compiler won't help much here if the types mismatch
			// subtly for method receivers.
			//
			// The key is that `avahi.EntryGroup` is a struct, not an interface.
			// The `mockAvahiEntryGroup` we defined is a struct that has methods with the same signature.
			// We can't directly cast `*mockAvahiEntryGroup` to `*avahi.EntryGroup`.
			//
			// We need an adapter or a way to have an actual *avahi.EntryGroup
			// whose methods call our mock. This is usually done by creating an actual
			// *avahi.EntryGroup (if possible without a real server) and then making its
			// callable methods delegate to our mock logic.
			//
			// Given the external library, the most robust way to mock `*avahi.EntryGroup`
			// when it's returned by `*avahi.Server` is to have an interface for `AvahiServer`
			// and `AvahiEntryGroup` in our own code, and then wrap the real `go-avahi` types.
			// Since that's a larger refactor:
			//
			// Alternative: The `groups map[string]*avahi.EntryGroup` stores these.
			// We can make `EntryGroupNewFunc` return a uniquely identifiable placeholder
			// and then, within the test, replace it in the map with our mock,
			// but this is getting complicated and breaks encapsulation.
			//
			// Let's try the direct approach: make mockAvahiEntryGroup satisfy the methods.
			// The type system won't allow returning *mockAvahiEntryGroup as *avahi.EntryGroup.
			//
			// The most straightforward way with current structure, without refactoring main code
			// to use interfaces for Avahi types, is to make mockAvahiEntryGroup embed
			// avahi.EntryGroup. This makes it an avahi.EntryGroup.
			// Then, our functions like IsEmptyFunc override the behavior.
			//
			// So, `mockAvahiEntryGroup` embedding `avahi.EntryGroup` is the way.
			return &mockEntryGroup.EntryGroup, nil // Return the embedded real EntryGroup
		},
	}

	egs := newEntryGroups(&avahi.Server{}) // Pass a real server, but it won't be used if EntryGroupNew is mocked
	// Override the server with our mock. This is a bit of a hack.
	// Ideally, newEntryGroups would take an interface.
	egs.avahiServer = (*avahi.Server)(unsafe.Pointer(mockServer)) // Unsafe, but common for mocking concrete external types

	containerID := "test_container_success"

	// Configure mockEntryGroup
	var isEmptyCallCount int
	mockEntryGroup.IsEmptyFunc = func() (bool, error) {
		isEmptyCallCount++
		if isEmptyCallCount == 1 { // First call in commit()
			t.Log("mockEntryGroup.IsEmpty called, returning false (not empty)")
			return false, nil
		}
		// This won't be called if commit logic is as expected (only one IsEmpty)
		t.Log("mockEntryGroup.IsEmpty called, returning true (empty)")
		return true, nil
	}
	mockEntryGroup.CommitFunc = func() error {
		t.Log("mockEntryGroup.Commit called")
		return nil
	}

	group, commitFn, err := egs.get(containerID)

	if err != nil {
		t.Fatalf("get() returned error: %v, expected nil", err)
	}
	if group == nil {
		t.Fatal("get() returned nil group, expected non-nil")
	}
	if commitFn == nil {
		t.Fatal("get() returned nil commitFn, expected non-nil")
	}

	// Check if the group returned is the one from the mock (via map)
	// This requires accessing egs.groups, which is fine for testing.
	storedGroup, ok := egs.groups[containerID]
	if !ok {
		t.Fatalf("group for %s not found in egs.groups", containerID)
	}
	// This comparison is tricky due to unsafe.Pointer. We expect `group` to be `&mockEntryGroup.EntryGroup`.
	// And `storedGroup` should also be `&mockEntryGroup.EntryGroup`.
	if storedGroup != &mockEntryGroup.EntryGroup {
		t.Errorf("storedGroup (%p) is not the expected mockEntryGroup.EntryGroup (%p)", storedGroup, &mockEntryGroup.EntryGroup)
	}
	if group != &mockEntryGroup.EntryGroup {
		t.Errorf("returned group (%p) is not the expected mockEntryGroup.EntryGroup (%p)", group, &mockEntryGroup.EntryGroup)
	}


	// Call commitFn and check behavior
	commitFn() // This will call egs.mutex.Unlock() and then group.IsEmpty() and group.Commit()

	if isEmptyCallCount < 1 {
		t.Error("expected mockEntryGroup.IsEmpty to be called at least once by commitFn, was called 0 times")
	}
	if !mockEntryGroup.commitCalled {
		t.Error("expected mockEntryGroup.Commit to be called by commitFn, but it wasn't")
	}

	// Ensure group is still in the map after successful get and commit
	if _, ok := egs.groups[containerID]; !ok {
		t.Errorf("group for %s was removed from egs.groups after commit; expected it to remain", containerID)
	}
	
	// Test getting the same group again (should not call EntryGroupNew)
	mockServer.EntryGroupNewFunc = func() (*avahi.EntryGroup, error) {
		t.Fatal("EntryGroupNew was called on second get() for the same containerID")
		return nil, errors.New("should not be called")
	}
	
	group2, commitFn2, err2 := egs.get(containerID)
	if err2 != nil {
		t.Fatalf("second get() returned error: %v, expected nil", err2)
	}
	if group2 == nil {
		t.Fatal("second get() returned nil group, expected non-nil")
	}
	if commitFn2 == nil {
		t.Fatal("second get() returned nil commitFn, expected non-nil")
	}
	if group2 != &mockEntryGroup.EntryGroup {
		t.Errorf("second get() returned group (%p), expected (%p)", group2, &mockEntryGroup.EntryGroup)
	}
	commitFn2() // Should also work
}

func TestEntryGroupsGet_EntryGroupNewFailure(t *testing.T) {
	// Suppress logging during tests
	originalLogger := log.Logger
	log.Logger = func(priority log.Priority, format string, args ...interface{}) {}
	defer func() { log.Logger = originalLogger }()

	expectedError := errors.New("avahi server error")
	mockServer := &mockAvahiServer{
		EntryGroupNewFunc: func() (*avahi.EntryGroup, error) {
			t.Log("mockAvahiServer.EntryGroupNew called, returning error")
			return nil, expectedError
		},
	}

	egs := newEntryGroups(&avahi.Server{}) // Real server, will be replaced
	egs.avahiServer = (*avahi.Server)(unsafe.Pointer(mockServer)) // Unsafe cast

	containerID := "test_container_fail"
	group, commitFn, err := egs.get(containerID)

	if err == nil {
		t.Fatal("get() did not return an error when EntryGroupNew failed")
	}
	if !errors.Is(err, expectedError) { // Check if it's the specific error or wraps it
	    // Check if the error message contains the expected error string,
	    // as fmt.Errorf in get() wraps the original error.
	    expectedErrStr := fmt.Sprintf("error creating new Avahi entry group for container %s: %s", containerID, expectedError.Error())
	    if err.Error() != expectedErrStr {
		    t.Fatalf("get() returned error '%v', expected to wrap '%v' or be '%s'", err, expectedError, expectedErrStr)
	    }
	}
	if group != nil {
		t.Fatalf("get() returned non-nil group (%v), expected nil when EntryGroupNew fails", group)
	}
	if commitFn == nil {
		t.Fatal("get() returned nil commitFn, expected non-nil even on failure")
	}

	// Call commitFn and check behavior (should be a no-op for group operations)
	// It should still unlock the mutex. To test this properly, we'd need to see the lock state,
	// or ensure no panic.
	recovered := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("commitFn panicked: %v", r)
				recovered = true
			}
		}()
		commitFn() // This should call egs.mutex.Unlock() but not touch a group
	}()
	if recovered {
		t.Fatal("Panic occurred in commitFn after EntryGroupNew failure")
	}


	// Verify that the internal e.groups map does not contain an entry for the container ID
	egs.mutex.Lock() // Need to lock to safely access groups map
	defer egs.mutex.Unlock()
	if _, ok := egs.groups[containerID]; ok {
		t.Errorf("egs.groups contains entry for %s, expected it to be absent after EntryGroupNew failure", containerID)
	}
}

// Note: The use of unsafe.Pointer is a workaround for the `go-avahi` library using concrete types.
// A more robust long-term solution would be to define interfaces for Avahi services within this project
// and use wrappers around the `go-avahi` types, allowing for conventional interface-based mocking.
// For this exercise, `unsafe.Pointer` is used to directly manipulate the server field.
// This requires `TestEntryGroupsGet_Success` and `TestEntryGroupsGet_EntryGroupNewFailure` to be in the same package `main`.

// Need to import "unsafe" for the mock server assignment.
import "unsafe"

/*
Further considerations for TestEntryGroupsGet_Success:
1. Test commitFn when IsEmpty returns true:
   - mockEntryGroup.IsEmptyFunc should return true, nil
   - mockEntryGroup.CommitFunc should NOT be called.
   - Add a flag `commitShouldBeCalled` and set it based on IsEmpty.

2. Test commitFn when IsEmpty returns an error:
   - mockEntryGroup.IsEmptyFunc should return false, errors.New("is empty error")
   - mockEntryGroup.CommitFunc should NOT be called.

3. Test commitFn when Commit returns an error:
    - mockEntryGroup.IsEmptyFunc returns false, nil
    - mockEntryGroup.CommitFunc returns errors.New("commit error")
    - Verify error is logged (hard without log capture) or that it doesn't panic.
*/

func TestEntryGroupsGet_Success_CommitLogicPaths(t *testing.T) {
	originalLogger := log.Logger
	log.Logger = func(priority log.Priority, format string, args ...interface{}) {
		// t.Logf("LOG: %s", fmt.Sprintf(format, args...)) // Optionally log to test output
	}
	defer func() { log.Logger = originalLogger }()

	containerID := "test_commit_paths"

	tests := []struct {
		name                  string
		isEmptyReturn         bool
		isEmptyError          error
		commitReturnError     error
		expectCommitCalled    bool
		setupMockEntryGroup func(*mockAvahiEntryGroup)
	}{
		{
			name:               "CommitCalled_WhenNotEmpty_NoError",
			isEmptyReturn:      false,
			isEmptyError:       nil,
			commitReturnError:  nil,
			expectCommitCalled: true,
		},
		{
			name:               "CommitNotCalled_WhenEmpty_NoError",
			isEmptyReturn:      true,
			isEmptyError:       nil,
			commitReturnError:  nil, // Should not matter
			expectCommitCalled: false,
		},
		{
			name:               "CommitNotCalled_WhenIsEmptyFails",
			isEmptyReturn:      false, // Should not matter
			isEmptyError:       errors.New("is empty failed"),
			commitReturnError:  nil, // Should not matter
			expectCommitCalled: false,
		},
		{
			name:               "CommitCalled_WhenNotEmpty_CommitReturnsError",
			isEmptyReturn:      false,
			isEmptyError:       nil,
			commitReturnError:  errors.New("commit failed"),
			expectCommitCalled: true, // Commit is still attempted
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockEntryGroup := &mockAvahiEntryGroup{}
			mockServer := &mockAvahiServer{
				EntryGroupNewFunc: func() (*avahi.EntryGroup, error) {
					return &mockEntryGroup.EntryGroup, nil
				},
			}
			egs := newEntryGroups(nil) // Pass nil, will be replaced
			egs.avahiServer = (*avahi.Server)(unsafe.Pointer(mockServer))


			mockEntryGroup.IsEmptyFunc = func() (bool, error) {
				t.Logf("[%s] mockEntryGroup.IsEmpty called, returning (%v, %v)", tc.name, tc.isEmptyReturn, tc.isEmptyError)
				return tc.isEmptyReturn, tc.isEmptyError
			}
			mockEntryGroup.CommitFunc = func() error {
				t.Logf("[%s] mockEntryGroup.Commit called, returning %v", tc.name, tc.commitReturnError)
				return tc.commitReturnError
			}
			mockEntryGroup.commitCalled = false // Reset for each sub-test run

			_, commitFn, err := egs.get(containerID)
			if err != nil {
				t.Fatalf("egs.get() failed: %v", err)
			}

			// Call commitFn
			commitFn()

			if mockEntryGroup.commitCalled != tc.expectCommitCalled {
				t.Errorf("expected Commit() call status to be %v, but got %v", tc.expectCommitCalled, mockEntryGroup.commitCalled)
			}
			
			// Clean up group for next iteration if containerID is the same
			// This is important because egs.groups persists across subtests if not reset
			delete(egs.groups, containerID) 
		})
	}
}

// Dummy main for testing if needed, not typical for _test.go files
// func main() {
// 	// This can be used to run tests with `go run .` if you add build tags
// 	// or temporarily change package to main for the actual code.
// 	// Usually, `go test` is the way.
// 	fmt.Println("This is a test file, run with 'go test'")
// }

// Ensure `avahi.EntryGroup` is actually part of `mockAvahiEntryGroup`
// for the unsafe cast to be less problematic.
var _ *avahi.EntryGroup = &(&mockAvahiEntryGroup{}).EntryGroup
var _ avahiServerInterface = &mockAvahiServer{} // Define this interface if we refactor
// var _ avahiEntryGroupInterface = &mockAvahiEntryGroup{} // Define this interface if we refactor

type avahiServerInterface interface {
    EntryGroupNew() (*avahi.EntryGroup, error)
    // Add other methods from avahi.Server if used
}

// If we had avahiEntryGroupInterface, it would look like:
// type avahiEntryGroupInterface interface {
// IsEmpty() (bool, error)
// Commit() error
// Reset() error
// AddService(iface int32, protocol int32, flags uint32, name string, stype string, domain string, host string, port uint16, txt [][]byte) error
// AddAddress(iface int32, protocol int32, flags uint32, name string, address string) error
// Free() // important for cleanup
// }
// The `Free()` method is on `avahi.EntryGroup`. If our commit logic or other logic ever calls `Free()`,
// our mock would need to implement it. Current code doesn't seem to call `Free()` on the group from `get`'s commit.
// It's typically called when a group is no longer needed at all.

// Final check of imports
var _ sync.Locker = &sync.Mutex{} // Used by egs.mutex

// To make unsafe.Pointer work for egs.avahiServer, we need to ensure that
// mockAvahiServer has a compatible layout if any fields were to be accessed
// directly from the avahi.Server pointer. Since we only call methods, it's generally
// safe as long as the method set is what's expected.
// The `(*avahi.Server)(unsafe.Pointer(mockServer))` cast is telling the compiler
// "trust me, mockServer can be treated as an *avahi.Server for method calls".
// This is true if mockAvahiServer implements the methods of avahi.Server that are used.
// In this case, only `EntryGroupNew`.
