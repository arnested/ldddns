package container_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/moby/moby/api/types/container"
	internalContainer "ldddns.arnested.dk/internal/container"
)

func containerJSON() (*container.InspectResponse, error) {
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
	var containerJSON *container.InspectResponse

	err = json.Unmarshal(byteValue, &containerJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling JSON test data: %w", err)
	}

	return containerJSON, nil
}

func containerData() (*internalContainer.Container, error) {
	containerJSON, err := containerJSON()
	if (err != nil) || (containerJSON == nil) {
		return nil, fmt.Errorf("getting JSON test data: %w", err)
	}

	data := internalContainer.Container{InspectResponse: *containerJSON}

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

func TestServicesEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ports    string
		expected int
	}{
		{
			name:     "valid http port",
			ports:    `{"80/tcp": [{"HostIp": "", "HostPort": ""}]}`,
			expected: 1,
		},
		{
			name:     "unknown service port - no service found",
			ports:    `{"9999/tcp": [{"HostIp": "", "HostPort": ""}]}`,
			expected: 0,
		},
		{
			name:     "unknown protocol type",
			ports:    `{"80/unknown": [{"HostIp": "", "HostPort": ""}]}`,
			expected: 0,
		},
		{
			name: "multiple ports with mixed results",
			ports: `{
				"80/tcp": [{"HostIp": "", "HostPort": ""}],
				"9999/tcp": [{"HostIp": "", "HostPort": ""}],
				"22/tcp": [{"HostIp": "", "HostPort": ""}]
			}`,
			expected: 2, // http and ssh are known services, 9999 is not
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			c := createTestContainerWithPorts(t, testCase.ports)
			services := c.Services()

			if len(services) != testCase.expected {
				t.Errorf("Expected %d services, got %d: %v", testCase.expected, len(services), services)
			}
		})
	}
}

func createTestContainerWithPorts(t *testing.T, ports string) internalContainer.Container {
	t.Helper()

	jsonData := fmt.Sprintf(`{
		"Id": "test",
		"Name": "/test",
		"NetworkSettings": {
			"Ports": %s,
			"Networks": {}
		},
		"Config": {
			"Env": [],
			"Labels": {}
		}
	}`, ports)

	var inspectResponse container.InspectResponse

	err := json.Unmarshal([]byte(jsonData), &inspectResponse)
	if err != nil {
		t.Fatalf("failed to unmarshal test data: %v", err)
	}

	return internalContainer.Container{InspectResponse: inspectResponse}
}

func TestIPAddressesEmpty(t *testing.T) {
	t.Parallel()

	// Test container with no network settings
	jsonData := `{
		"Id": "test",
		"Name": "/test",
		"NetworkSettings": {
			"Ports": {},
			"Networks": {}
		},
		"Config": {
			"Env": [],
			"Labels": {}
		}
	}`

	var inspectResponse container.InspectResponse

	err := json.Unmarshal([]byte(jsonData), &inspectResponse)
	if err != nil {
		t.Fatalf("failed to unmarshal test data: %v", err)
	}

	c := internalContainer.Container{InspectResponse: inspectResponse}
	ips := c.IPAddresses()

	if len(ips) != 0 {
		t.Errorf("Expected 0 IP addresses for container with no networks, got %d", len(ips))
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
