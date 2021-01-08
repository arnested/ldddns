# Local Docker Development DNS

A systemd service that will monitor your Docker host and provide
DNS names for containers with a `VIRTUAL_HOST` environment variable.

The service broadcasts the domain names using multicast DNS.

A limitation of this is that domains name can only be on the `.local`
TLD and only have one level below the TLD. A benefit is that you don't
have change your DNS server or configure stuff i `/etc/resolv` or
similar.

The service will rewrite hostnames in `VIRTUAL_HOST` to match
this. I.e. `my.fancy.site` will be rewritten to `my-fancy.local`.

If the containers also have exposed ports (and the ports can be looked
up in `/etc/services`) the service will also broadcast the
service/domain for service discovery (i.e. `_https._tcp.` for
https://my-fancy.local).

## Install

For Pop!_OS, Ubuntu, Debian and the like, download the `.deb` package
file from the [latest
release](https://github.com/arnested/ldddns/releases/latest) and open
it or run:

```console
sudo dpkg -i ldddns_0.0.38_linux_amd64.deb
```

Or just run the following command which will download and install the
latest package for you:

```console
curl -fsSL https://ldddns.arnested.dk/install.sh | bash
```

For other distributions download the binary from the [latest
release](https://github.com/arnested/ldddns/releases/latest) and
create a systemd service unit file yourself based on
[`ldddns.service`](https://github.com/arnested/ldddns/blob/main/ldddns.service).

### Updates

When you install the package it will add an APT source list so you
will receive future updates to `ldddns` along with your other system
updates.
