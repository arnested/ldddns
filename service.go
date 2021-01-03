package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/coreos/go-systemd/unit"
)

func installService() {
	version := serverVersion()

	executable, err := os.Executable()
	if err != nil {
		panic(fmt.Errorf("could not find path of executable: %w", err))
	}

	serviceUnit := unit.Serialize([]*unit.UnitOption{
		unit.NewUnitOption("Unit", "Description", "Local Docker Development DNS"),
		unit.NewUnitOption("Unit", "BindTo", "docker.service"),
		unit.NewUnitOption("Unit", "After", "docker.service"),
		unit.NewUnitOption("Service", "Type", "notify"),
		unit.NewUnitOption("Service", "Environment", "DOCKER_API_VERSION="+version),
		unit.NewUnitOption("Service", "ExecStart", executable+" service"),
		unit.NewUnitOption("Service", "SuccessExitStatus", "15"),
		unit.NewUnitOption("Service", "Restart", "on-failure"),
		unit.NewUnitOption("Install", "WantedBy", "docker.service"),
	})

	_, err = io.Copy(os.Stdout, serviceUnit)
	if err != nil {
		panic(fmt.Errorf("could not output service unit: %w", err))
	}
}

func serverVersion() string {
	var out bytes.Buffer

	cmd := exec.Command("docker", "version", "--format", "{{ .Server.APIVersion }}")
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	return strings.Trim(out.String(), "\n")
}
