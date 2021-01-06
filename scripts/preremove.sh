#!/bin/sh

set -e

if /bin/systemctl is-active --quiet ldddns.service; then
    /bin/systemctl stop ldddns.service
fi

if /bin/systemctl is-enabled --quiet ldddns.service; then
    /bin/systemctl disable --now ldddns.service;
fi
