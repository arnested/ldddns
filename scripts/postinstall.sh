#!/bin/sh

set -e

docker_version=$(/usr/bin/docker version --format '{{ .Server.APIVersion }}')

/usr/bin/sed -i "s/\(DOCKER_API_VERSION=\).*/\1${docker_version}/" /etc/default/ldddns

/bin/systemctl daemon-reload

if /bin/systemctl is-active --quiet ldddns.service; then
    /bin/systemctl restart ldddns.service
fi

if ! /bin/systemctl is-enabled --quiet ldddns.service; then
    /bin/systemctl enable --now ldddns.service;
fi
