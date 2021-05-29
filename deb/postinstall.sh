#!/bin/sh

set -e

docker_version=$(/usr/bin/docker version --format '{{ .Server.APIVersion }}')

# Work around systemd reporting dropins being changed on disk.
# See https://github.com/systemd/systemd/issues/17730
# Fixed in 248-rc3 (https://github.com/systemd/systemd/pull/18869).
if [ "$(systemd --version | head -1 | sed -e 's/systemd \([0-9]*\).*/\1/')" -lt "248" ]; then
    for dropin in $(systemctl cat ldddns.service | grep '^# /etc/systemd/system/ldddns.service.d/' | cut -c 3-); do
        [ -e "${dropin}" ] && touch "${dropin}"
    done
fi

mkdir --parents /usr/lib/systemd/system/ldddns.service.d
printf "[Service]\nEnvironment=DOCKER_API_VERSION=%s\n" "${docker_version}" > /usr/lib/systemd/system/ldddns.service.d/docker-version.conf

/bin/systemctl daemon-reload

if /bin/systemctl is-active --quiet ldddns.service; then
    /bin/systemctl restart ldddns.service
fi

if ! /bin/systemctl is-enabled --quiet ldddns.service; then
    /bin/systemctl enable --now ldddns.service;
fi
