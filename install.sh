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

    echo -n "Finding latest package name..."
    package=$(curl --proto =https --fail -sSL "https://github.com/arnested/ldddns/releases/latest/download/checksums.txt" | grep -i "$(uname -s)" | grep "$(dpkg --print-architecture)" | grep \.deb | awk '{print $2}')
    echo " ${package}"

    echo "Downloading ${package}..."
    curl --proto =https --fail --location --progress-bar --output "${tmpdir}/${package}" "https://github.com/arnested/ldddns/releases/latest/download/${package}"

    echo "Installing ${package}..."
    pkexec dpkg -i "${tmpdir}/${package}"
}

ldddns_install
