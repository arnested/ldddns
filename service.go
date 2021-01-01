package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"strings"
)

func installService() {
	serviceUnit := `[Unit]
Description=Local Docker Development DNS
BindTo=docker.service
After=docker.service

[Service]
Type=notify
Environment="DOCKER_API_VERSION={{ .Version }}"
ExecStart={{ .Executable }} service
SuccessExitStatus=15
Restart=on-failure

[Install]
WantedBy=docker.service
`

	version := serverVersion()

	executable, err := os.Executable()
	if err != nil {
		panic(fmt.Errorf("could not find path of executable: %w", err))
	}

	data := struct {
		Version    string
		Executable string
	}{
		Version:    version,
		Executable: executable,
	}

	tmpl, _ := template.New("test").Parse(serviceUnit)
	_ = tmpl.Execute(os.Stdout, data)
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
