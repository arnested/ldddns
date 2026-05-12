#!/usr/bin/env bash

# shellcheck disable=SC2292
if [ -z "${BASH}" ]; then
    echo >&2 Please run the install with bash.
    exit 1
fi

aptget="$(command -v apt-get || true)"
aptdcon="$(command -v aptdcon || true)"
pkexec="$(command -v pkexec || true)"

if [[ ! -x "${aptdcon}" && ! -x "${aptget}" ]]; then
    echo >&2 Install only runs on Debian based distributions.
    exit 2
fi

set -euo pipefail

ldddns_install() {
    tmpdir="$(mktemp --directory)"

    # Make a cleanup function
    cleanup() {
        rm --recursive --force -- "${tmpdir}"
    }
    trap cleanup EXIT
    setfacl -m u:_apt:rx "${tmpdir}"

    echo -n "Finding latest package name..."

    uname="$(uname -s)"
    arch="$(dpkg --print-architecture)"

    package=$(curl --proto =https --fail --location --silent --show-error "https://github.com/arnested/ldddns/releases/latest/download/checksums.txt" | grep --ignore-case "${uname}" | grep "${arch}" | grep \.deb | awk '{print $2}')
    echo " ${package}"

    echo "Downloading ${package}..."
    curl --proto =https --fail --location --progress-bar --output "${tmpdir}/${package}" "https://github.com/arnested/ldddns/releases/latest/download/${package}"

    echo "Installing ${package}..."

    if [[ -x "${aptdcon}"  ]]; then
        yes | aptdcon --hide-terminal --install "${tmpdir}/${package}" > /dev/null
    elif [[ -x "${pkexec}"  ]]; then
        pkexec apt-get install  "${tmpdir}/${package}"
    else
        sudo apt-get install  "${tmpdir}/${package}"
    fi
}

ldddns_install
