#!/usr/bin/env bash

if [ -z "$BASH" ] ;then echo Please run this with bash; exit 1; fi

set -euo pipefail

ldddns_install() {
    set -euo pipefail

    tmpdir="$(mktemp -d)"

    # Make a cleanup function
    cleanup() {
        rm --recursive --force -- "${tmpdir}"
    }
    trap cleanup EXIT

    echo Downloading ldddns binary
    curl --proto =https --fail --location --progress-bar --output "${tmpdir}/ldddns" "https://github.com/arnested/ldddns/releases/latest/download/ldddns_$(uname -s)_$(uname -m)"

    chmod +x "${tmpdir}/ldddns"

    install_dir=/usr/local/libexec/ldddns

    echo Making directory \(${install_dir}\) for installing service binary
    mkdir -p "${install_dir}"

    echo Installing service binary in ${install_dir}
    mv "${tmpdir}/ldddns" "${install_dir}"

    echo Generating systemd service unit
    "${install_dir}/ldddns" > "${tmpdir}/ldddns.service"

    echo Installing systemd service unit in /etc/systemd/system/ldddns.service
    mv "${tmpdir}/ldddns.service" /etc/systemd/system/ldddns.service

    echo Reloading systemd daemon
    systemctl daemon-reload

    if systemctl is-active --quiet ldddns.service; then
        echo Found existing, running ldddns.service - restarting it
        systemctl restart ldddns.service
    fi

    echo Enabling systemd service
    systemctl enable --now ldddns.service;
}

pkexec bash -c "$(declare -f ldddns_install) ; ldddns_install"
