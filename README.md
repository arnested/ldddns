# Local Docker Development DNS

A systemd service that will monitor your Docker host and provide
DNS names for containers with a `VIRTUAL_HOST` environment variable.

The domain names will be broadcast using multicast DNS.

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

Download the `.deb` file and run:

```console
sudo dpkg -i ldddns_0.0.16_linux_amd64.deb
```

Or just run the following command which will download and install the
latest package for you:

```console
curl -fsSL https://ldddns.arnested.dk/install.sh | bash
```
