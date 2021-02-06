# Local Docker Development DNS

A systemd service that will monitor your Docker host and provide
DNS names for the containers.

The service broadcasts the domain names using multicast DNS
(a.k.a. mDNS, zeroconf, Bonjour, Avahi).

A limitation of this is that domains name can only be on the `.local`
TLD and only have one level below the TLD. A benefit is that you don't
have to change your DNS server or configure stuff in `/etc/resolv` or
similar.

If the containers also have exposed ports (and the ports can be looked
up in `/etc/services`) the service will also broadcast the
service/domain for service discovery. I.e., `_https._tcp.` for
https://my-fancy.local (a.k.a. DNS-SD). Only _one_ domain name can be
broadcast per service per container.

Per default domain names will be generated from the `VIRTUAL_HOST`
environment variable is present (several hostnames can be separated by
space or comma) and from the container name.

If the hostnames do not fulfill the rule of being on the `.local` TLD
and have only one level below the service will rewrite it.
I.e. `my.fancy.com` will be rewritten to `my-fancy.local`.

## Configuration

You can configure where the service should look for hostnames:

* Environment variables (configured with `env:<VAR_NAME>`) - several
  hostnames can be separated by spaces or commas.
* Container name (configured with `containerName`) - the container
  name will never be a valid hostname to begin with, but as mentioned
  the `ldddns` will rewrite it into one.
* Labels (configured with `label:<label.name>`) - several hostnames
  can be separated by spaces or commas.

You configure it be setting the environment variable
`LDDDNS_HOSTNAME_LOOKUP` in a systemd unit override file.

For example, you could create a file named
`/etc/systemd/system/ldddns.service.d/override.conf` with the content:

```ini
[Service]
Environment=LDDDNS_HOSTNAME_LOOKUP=env:VIRTUAL_HOST,label:org.example.my.hostname,env:OTHER_VAR,containerName
```

This will create a hostname for all hostnames in the `VIRTUAL_HOST`
environment variable, the `org.example.my.hostname` label, the
`OTHER_VAR` environment variable, and the container name.

The first hostname found will be broadcast as a DNS-SD service.

The default configuration is the equivalent of setting:

```ini
[Service]
Environment=LDDDNS_HOSTNAME_LOOKUP=env:VIRTUAL_HOST,containerName
```

## Install

For Pop!_OS, Ubuntu, Debian and the like, download the `.deb` package
file from the [latest
release](https://github.com/arnested/ldddns/releases/latest) and open
it or run:

```console
sudo dpkg -i ldddns_0.0.71_linux_amd64.deb
```

Or just run the following command which will download and install the
latest package for you:

```console
curl -fsSL https://ldddns.arnested.dk/install.sh | bash
```

For other distributions download the binary from the [latest
release](https://github.com/arnested/ldddns/releases/latest) and
create a systemd service unit file yourself based on
[`ldddns.service`](https://github.com/arnested/ldddns/blob/main/systemd/ldddns.service).

### Updates

When you install the package it will add an APT source list so, you
will receive future updates to `ldddns` along with your other system
updates.

## Keeping an eye on things

You can get the status of the service by running:

```console
sudo systemctl status ldddns.service
● ldddns.service - Local Docker Development DNS
     Loaded: loaded (/lib/systemd/system/ldddns.service; enabled; vendor preset: enabled)
     Active: active (running) since Fri 2021-01-08 09:29:46 CET; 44min ago
   Main PID: 87715 (ldddns)
      Tasks: 8 (limit: 47858)
     Memory: 5.2M
     CGroup: /system.slice/ldddns.service
             └─87715 /usr/libexec/ldddns

jan 08 09:29:46 pop-os systemd[1]: Starting Local Docker Development DNS...
jan 08 09:29:46 pop-os ldddns[87715]: Starting ldddns v0.0.71...
jan 08 09:29:46 pop-os systemd[1]: Started Local Docker Development DNS.
jan 08 10:13:52 pop-os ldddns[87715]: Rewrote hostname from "my.fancy.com" to "my-fancy.local"
jan 08 10:13:52 pop-os ldddns[87715]: added address for "my-fancy.local" pointing to "172.19.0.3"
jan 08 10:13:52 pop-os ldddns[87715]: added service "_http._tcp" pointing to "my-fancy.local"
```

Or follow the log with:

```console
sudo journalctl --follow --unit ldddns.service
```

## Bugs, thoughts, and comments

Bugs, thoughts, and comments are welcome.

Feel free to get in touch at [GitHub
Issues](https://github.com/arnested/ldddns/issues) and [GitHub
Discussions](https://github.com/arnested/ldddns/discussions).
