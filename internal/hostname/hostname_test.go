package hostname_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/docker/docker/api/types"
	"ldddns.arnested.dk/internal/container"
	"ldddns.arnested.dk/internal/hostname"
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
	if (err != nil) || (containerJSON == nil) {
		return nil, fmt.Errorf("getting JSON test data: %w", err)
	}

	data := container.Container{ContainerJSON: *containerJSON}

	return &data, nil
}

//nolint:cyclop
func TestHostnames(t *testing.T) {
	t.Parallel()

	data, err := containerData()
	if err != nil {
		t.Fatalf("getting test data: %s", err)
	}

	hostnames, err := hostname.Hostnames(*data, []string{
		"env:VIRTUAL_HOST",
		"containerName",
		"env:VIRTUAL_HOST", // we repeat VIRTUAL_HOST to check if we remove duplicates
		"label:com.docker.compose.service",
	})
	if err != nil {
		t.Fatalf("Unexpected error getting hostnames: %s", err)
	}

	expected := []string{"foobar.local", "baz.local", "foobar-client-1.local", "client.local"}

	if len(hostnames) != len(expected) {
		t.Errorf("Expected %d hostnames, got %d hostnames.", len(expected), len(hostnames))
	}

	if len(hostnames) > 0 && hostnames[0] != expected[0] {
		t.Errorf("Expected first hostname to be %q, got %q.", expected[0], hostnames[0])
	}

	if len(hostnames) > 1 && hostnames[1] != expected[1] {
		t.Errorf("Expected second hostname to be %q, got %q.", expected[1], hostnames[1])
	}

	if len(hostnames) > 2 && hostnames[2] != expected[2] {
		t.Errorf("Expected third hostname to be %q, got %q.", expected[2], hostnames[2])
	}

	if len(hostnames) > 3 && hostnames[3] != expected[3] {
		t.Errorf("Expected fourth hostname to be %q, got %q.", expected[3], hostnames[3])
	}
}

func TestRewriteHostname(t *testing.T) {
	t.Parallel()

	testdata := []struct {
		in  string
		out string
	}{
		{"example.com", "example.local"},
		{"example87.com", "example87.local"},
		{"foo_bar", "foo-bar.local"},
		{"foo__bar", "foo-bar.local"},
		{"foo_-_bar", "foo-bar.local"},
		{"_foo_bar_", "foo-bar.local"},
		{"-foo_bar-", "foo-bar.local"},
		{"blåbærgrød", "xn--blbrgrd-fxak7p.local"},
		{"xn--blbrgrd-fxak7p.local", "xn--blbrgrd-fxak7p.local"},
	}

	for _, tt := range testdata {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			if s := hostname.RewriteHostname(tt.in); s != tt.out {
				t.Errorf("got %q from %q, want %q", s, tt.in, tt.out)
			}
		})
	}
}

func FuzzRewriteHostname(f *testing.F) {
	f.Add("example.com")
	f.Add("example87.com")
	f.Add("foo_bar")
	f.Add("foo__bar")
	f.Add("foo_-_bar")
	f.Add("_foo_bar_")
	f.Add("-foo_bar-")
	f.Add("blåbærgrød")
	f.Add("xn--blbrgrd-fxak7p.local")

	f.Fuzz(func(_ *testing.T, a string) {
		hostname.RewriteHostname(a)
	})
}
