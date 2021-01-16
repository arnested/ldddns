#!/bin/sh

set -e

if [ -f /usr/lib/systemd/system/ldddns.service.d/docker-version.conf ]; then
    rm /usr/lib/systemd/system/ldddns.service.d/docker-version.conf
fi

if [ -d /usr/lib/systemd/system/ldddns.service.d ]; then
    rmdir --ignore-fail-on-non-empty /usr/lib/systemd/system/ldddns.service.d
fi

/bin/systemctl daemon-reload
