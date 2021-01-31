#!/usr/bin/env bash

if [ -z "${BASH}" ]; then
    echo >&2 Please run the install with bash.
    exit 1
fi

set -euo pipefail

ldddns_install() {
    tmpdir="$(mktemp --directory)"

    # Make a cleanup function
    cleanup() {
        rm --recursive --force -- "${tmpdir}"
    }
    trap cleanup EXIT

    echo -n "Finding latest package name..."
    package=$(curl --proto =https --fail --location --silent --show-error "https://github.com/arnested/ldddns/releases/latest/download/checksums.txt" | grep --ignore-case "$(uname -s)" | grep "$(dpkg --print-architecture)" | grep \.deb | awk '{print $2}')
    echo " ${package}"

    echo "Downloading ${package}..."
    curl --proto =https --fail --location --progress-bar --output "${tmpdir}/${package}" "https://github.com/arnested/ldddns/releases/latest/download/${package}"

    echo "Installing ${package}..."
    yes | aptdcon --hide-terminal --install "${tmpdir}/${package}" > /dev/null
}

ldddns_install
