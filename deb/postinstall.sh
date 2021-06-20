#!/bin/sh

set -e

/bin/systemctl daemon-reload

if /bin/systemctl is-active --quiet ldddns.service; then
    /bin/systemctl restart ldddns.service
fi

if ! /bin/systemctl is-enabled --quiet ldddns.service; then
    /bin/systemctl enable --now ldddns.service;
fi
