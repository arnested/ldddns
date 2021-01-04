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
		unit.NewUnitOption("Service", "DynamicUser", "yes"),
		unit.NewUnitOption("Service", "SupplementaryGroups", "docker"),
		unit.NewUnitOption("Service", "CapabilityBoundingSet", ""),
		unit.NewUnitOption("Service", "DevicePolicy", "closed"),
		unit.NewUnitOption("Service", "IPAddressDeny", "any"),
		unit.NewUnitOption("Service", "LockPersonality", "yes"),
		unit.NewUnitOption("Service", "MemoryDenyWriteExecute", "yes"),
		unit.NewUnitOption("Service", "NoNewPrivileges", "yes"),
		unit.NewUnitOption("Service", "PrivateDevices", "yes"),
		unit.NewUnitOption("Service", "PrivateNetwork", "yes"),
		unit.NewUnitOption("Service", "PrivateUsers", "yes"),
		unit.NewUnitOption("Service", "ProtectClock", "yes"),
		unit.NewUnitOption("Service", "ProtectControlGroups", "yes"),
		unit.NewUnitOption("Service", "ProtectHome", "yes"),
		unit.NewUnitOption("Service", "ProtectHostname", "yes"),
		unit.NewUnitOption("Service", "ProtectKernelLogs", "yes"),
		unit.NewUnitOption("Service", "ProtectKernelModules", "yes"),
		unit.NewUnitOption("Service", "ProtectKernelTunables", "yes"),
		unit.NewUnitOption("Service", "RestrictAddressFamilies", "AF_UNIX"),
		unit.NewUnitOption("Service", "RestrictNamespaces", "yes"),
		unit.NewUnitOption("Service", "RestrictRealtime", "yes"),
		unit.NewUnitOption("Service", "SystemCallArchitectures", "native"),
		unit.NewUnitOption("Service", "SystemCallErrorNumber", "EPERM"),
		unit.NewUnitOption("Service", "SystemCallFilter", "@system-service"),
		unit.NewUnitOption("Service", "UMask", "0777"),
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
