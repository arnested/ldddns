#!/bin/sh

set -e

docker_version=$(/usr/bin/docker version --format '{{ .Server.APIVersion }}')

mkdir --parents /usr/lib/systemd/system/ldddns.service.d
printf "[Service]\nEnvironment=DOCKER_API_VERSION=%s\n" "${docker_version}" > /usr/lib/systemd/system/ldddns.service.d/docker-version.conf

# Remove config file from previous versions.
if [ -f /etc/default/ldddns ]; then
    rm /etc/default/ldddns
fi

/bin/systemctl daemon-reload

if /bin/systemctl is-active --quiet ldddns.service; then
    /bin/systemctl restart ldddns.service
fi

if ! /bin/systemctl is-enabled --quiet ldddns.service; then
    /bin/systemctl enable --now ldddns.service;
fi
