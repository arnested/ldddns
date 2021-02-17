package container_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/docker/docker/api/types"
	"ldddns.arnested.dk/internal/container"
)

func containerJSON() (*types.ContainerJSON, error) {
	jsonFile, err := os.Open("../../testdata/container.json")
	if err != nil {
		return nil, fmt.Errorf("opening JSON test data: %w", err)
	}

	defer jsonFile.Close()

	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		return nil, fmt.Errorf("reading JSON test data: %w", err)
	}

	// we initialize our Users array
	var containerJSON *types.ContainerJSON

	err = json.Unmarshal(byteValue, &containerJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling JSON test data: %w", err)
	}

	return containerJSON, nil
}

func containerData() (*container.Container, error) {
	containerJSON, err := containerJSON()
	if err != nil {
		return nil, fmt.Errorf("getting JSON test data: %w", err)
	}

	data := container.Container{ContainerJSON: *containerJSON}

	return &data, nil
}

func TestName(t *testing.T) {
	t.Parallel()

	data, err := containerData()
	if err != nil {
		t.Fatalf("getting test data: %s", err)
	}

	name := data.Name()
	expected := "foobar_client_1"

	if name != expected {
		t.Errorf("Expected container name %q, got %q.", expected, name)
	}
}

func TestIPAddresses(t *testing.T) {
	t.Parallel()

	data, err := containerData()
	if err != nil {
		t.Fatalf("getting test data: %s", err)
	}

	expected := []string{"172.18.0.4"}
	IPAddresses := data.IPAddresses()

	if len(IPAddresses) != len(expected) {
		t.Errorf("Expected %d IP address, got %d IP addresses.", len(expected), len(IPAddresses))
	}

	if len(IPAddresses) > 0 && IPAddresses[0] != expected[0] {
		t.Errorf("Expected first IP address to be %q, got %q.", expected[0], IPAddresses[0])
	}
}

func TestServices(t *testing.T) {
	t.Parallel()

	data, err := containerData()
	if err != nil {
		t.Fatalf("getting test data: %s", err)
	}

	expectedService := "_http._tcp"
	expectedPort := uint16(80)

	services := data.Services()

	if _, ok := services[expectedService]; !ok {
		t.Errorf("Expected a %q service, none found", expectedService)
	}

	if _, ok := services[expectedService]; ok && services[expectedService] != expectedPort {
		t.Errorf(
			"Expected %q service to be on port %d, got port %d",
			expectedService,
			expectedPort,
			services[expectedService],
		)
	}
}

func TestHostnamesFromEnv(t *testing.T) {
	t.Parallel()

	data, err := containerData()
	if err != nil {
		t.Fatalf("getting test data: %s", err)
	}

	expected := []string{"foobar.local", "baz.docker"}
	hostnames := data.HostnamesFromEnv("VIRTUAL_HOST")

	if len(hostnames) != len(expected) {
		t.Errorf("Expected %d hostnames, got %d hostnames.", len(expected), len(hostnames))
	}

	if len(hostnames) > 0 && hostnames[0] != expected[0] {
		t.Errorf("Expected first hostname to be %q, got %q.", expected[0], hostnames[0])
	}

	if len(hostnames) > 1 && hostnames[1] != expected[1] {
		t.Errorf("Expected second hostname to be %q, got %q.", expected[1], hostnames[1])
	}

	noHostnames := data.HostnamesFromEnv("NON_EXISTING_ENV_VAR")

	if len(noHostnames) != 0 {
		t.Errorf("Didn't expected any hostnames from `NON_EXISTING_ENV_VAR`, got %q.", noHostnames)
	}
}

func TestHostnamesFromLabel(t *testing.T) {
	t.Parallel()

	data, err := containerData()
	if err != nil {
		t.Fatalf("getting test data: %s", err)
	}

	expected := []string{"client"}
	hostnames := data.HostnamesFromLabel("com.docker.compose.service")

	if len(hostnames) != len(expected) {
		t.Errorf("Expected %d hostnames, got %d hostnames.", len(expected), len(hostnames))
	}

	if len(hostnames) > 0 && hostnames[0] != expected[0] {
		t.Errorf("Expected first hostname to be %q, got %q.", expected[0], hostnames[0])
	}

	noHostnames := data.HostnamesFromLabel("org.example.non-existing")

	if len(noHostnames) != 0 {
		t.Errorf("Didn't expected any hostnames from `NON_EXISTING_ENV_VAR`, got %q.", noHostnames)
	}
}
