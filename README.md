# Local Docker Development DNS

A systemd service that will monitor your Docker host and provide
DNS names for containers with a `VIRTUAL_HOST` environment variable.

The service broadcasts the domain names using multicast DNS
(a.k.a. mDNS, zeroconf, bounjour, avahi).

A limitation of this is that domains name can only be on the `.local`
TLD and only have one level below the TLD. A benefit is that you don't
have to change your DNS server or configure stuff in `/etc/resolv` or
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
sudo dpkg -i ldddns_0.0.41_linux_amd64.deb
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

## Keeping an eye on things

You can get the status of the service by running:

```console
$ sudo systemctl status ldddns.service
● ldddns.service - Local Docker Development DNS
     Loaded: loaded (/lib/systemd/system/ldddns.service; enabled; vendor preset: enabled)
     Active: active (running) since Fri 2021-01-08 09:29:46 CET; 44min ago
   Main PID: 87715 (ldddns)
      Tasks: 8 (limit: 47858)
     Memory: 5.2M
     CGroup: /system.slice/ldddns.service
             └─87715 /usr/libexec/ldddns

jan 08 09:29:46 pop-os systemd[1]: Starting Local Docker Development DNS...
jan 08 09:29:46 pop-os ldddns[87715]: Starting ldddns 0.0.41...
jan 08 09:29:46 pop-os systemd[1]: Started Local Docker Development DNS.
jan 08 10:13:52 pop-os ldddns[87715]: added address for "my-fancy.local" pointing to "172.19.0.3"
```

Or follow the log with:

```console
$ sudo journalctl --follow --unit ldddns.service
```

## Bugs, thoughts, and comments

Bugs, thoughts, and comments are welcome.

You can use the [GitHub
Issues](https://github.com/arnested/ldddns/issues) and/or [GitHub
Discussions](https://github.com/arnested/ldddns/discussions).
