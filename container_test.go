package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/holoplot/go-avahi"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

const (
	testContainerID = "abc123"
	testTimeout     = 5 * time.Second

	inspectWithoutIPs = `{"Id":"abc123","Name":"/test","Config":{"Labels":{}},"NetworkSettings":{"Networks":{}}}`
	inspectOneoff     = `{"Id":"abc123","Name":"/test",` +
		`"Config":{"Labels":{"com.docker.compose.oneoff":"True"}},` +
		`"NetworkSettings":{"Networks":{"bridge":{"IPAddress":"172.17.0.2"}}}}`
)

// fakeDocker is a fake Docker API. It answers container list and inspect
// requests and streams the events it reads from its events channel.
type fakeDocker struct {
	// inspect is the inspect response body. If it is empty, inspect
	// requests get a 404.
	inspect string
	// containers is the number of containers that list requests return.
	containers int
	// onInspect runs before each inspect response.
	onInspect func()
	events    chan string
	inspected atomic.Int32
}

func (f *fakeDocker) client(t *testing.T) *client.Client {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{version}/containers/json", f.list)
	mux.HandleFunc("GET /{version}/containers/{id}/json", f.inspectContainer)
	mux.HandleFunc("GET /{version}/events", f.streamEvents)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	docker, err := client.New(client.WithHost(server.URL), client.WithAPIVersion(client.MaxAPIVersion))
	if err != nil {
		t.Fatalf("creating docker client: %v", err)
	}

	t.Cleanup(func() { _ = docker.Close() })

	return docker
}

func (f *fakeDocker) list(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	fmt.Fprint(writer, "[")

	for i := range f.containers {
		if i > 0 {
			fmt.Fprint(writer, ",")
		}

		fmt.Fprintf(writer, `{"Id":"%s-%d"}`, testContainerID, i)
	}

	fmt.Fprint(writer, "]")
}

func (f *fakeDocker) inspectContainer(writer http.ResponseWriter, request *http.Request) {
	f.inspected.Add(1)

	if f.onInspect != nil {
		f.onInspect()
	}

	if f.inspect == "" {
		http.NotFound(writer, request)

		return
	}

	writer.Header().Set("Content-Type", "application/json")
	fmt.Fprint(writer, f.inspect)
}

func (f *fakeDocker) streamEvents(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)

	controller := http.NewResponseController(writer)
	_ = controller.Flush()

	for {
		select {
		case <-request.Context().Done():
			return
		case event := <-f.events:
			fmt.Fprintln(writer, event)

			_ = controller.Flush()
		}
	}
}

func TestHandleContainerStopEvents(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"destroy", "die", "kill", "pause"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()

			egs := newEntryGroups(nil)

			// A nil Docker client proves that stop events make no Docker calls.
			err := handleContainer(t.Context(), nil, testContainerID, egs, events.Action(status), Config{})
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}

			if len(egs.groups) != 0 {
				t.Error("Expected stop events to create no entry group")
			}
		})
	}
}

func TestHandleContainerStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inspect   string
		expectErr bool
	}{
		{name: "without IP addresses", inspect: inspectWithoutIPs, expectErr: false},
		{name: "oneoff", inspect: inspectOneoff, expectErr: false},
		{name: "inspect fails", inspect: "", expectErr: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeDocker{inspect: testCase.inspect}
			egs := newEntryGroups(nil)
			config := Config{IgnoreDockerComposeOneoff: true}

			err := handleContainer(t.Context(), fake.client(t), testContainerID, egs, "start", config)
			if (err != nil) != testCase.expectErr {
				t.Errorf("Expected error %v, got %v", testCase.expectErr, err)
			}

			if len(egs.groups) != 0 {
				t.Error("Expected no entry group")
			}
		})
	}
}

func TestHandleExistingContainers(t *testing.T) {
	t.Parallel()

	fake := &fakeDocker{inspect: inspectWithoutIPs, containers: 2}

	handleExistingContainers(t.Context(), Config{}, fake.client(t), newEntryGroups(nil))

	if got := fake.inspected.Load(); got != 2 {
		t.Errorf("Expected 2 inspected containers, got %d", got)
	}
}

func TestHandleExistingContainersStopsOnCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	fake := &fakeDocker{inspect: inspectWithoutIPs, containers: 2, onInspect: cancel}

	handleExistingContainers(ctx, Config{}, fake.client(t), newEntryGroups(nil))

	if got := fake.inspected.Load(); got != 1 {
		t.Errorf("Expected 1 inspected container before the cancel, got %d", got)
	}
}

func TestListen(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	inspected := make(chan struct{}, 1)
	fake := &fakeDocker{
		inspect:   inspectWithoutIPs,
		events:    make(chan string, 1),
		onInspect: func() { inspected <- struct{}{} },
	}
	docker := fake.client(t)

	done := make(chan struct{})

	go func() {
		defer close(done)

		listen(ctx, Config{}, docker, newEntryGroups(nil), time.Now())
	}()

	fake.events <- `{"Type":"container","Action":"start","Actor":{"ID":"abc123"}}`

	select {
	case <-inspected:
	case <-time.After(testTimeout):
		t.Fatal("Expected listen to handle the start event")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("Expected listen to return when the context is cancelled")
	}
}

func TestLogStateChanges(t *testing.T) {
	t.Parallel()

	states := make(chan avahi.EntryGroupState, 3)
	states <- avahi.EntryGroupState{State: avahi.EntryGroupCollision, Error: ""}

	states <- avahi.EntryGroupState{State: avahi.EntryGroupFailure, Error: "failed"}

	states <- avahi.EntryGroupState{State: avahi.EntryGroupEstablished, Error: ""}

	close(states)

	// logStateChanges must return when the channel closes.
	logStateChanges(testContainerID, states)
}

func TestExitAfterShutdownTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	exitCode := -1

	exitAfterShutdownTimeout(ctx, time.Millisecond, func(code int) { exitCode = code })

	if exitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", exitCode)
	}
}
