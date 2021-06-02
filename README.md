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
https://my-example.local (a.k.a. DNS-SD). Only _one_ domain name can
be broadcast per service per container.

Per default domain names will be generated from the `VIRTUAL_HOST`
environment variable is present (several hostnames can be separated by
space or comma) and from the container name.

If the hostnames do not fulfill the rule of being on the `.local` TLD
and have only one level below the service will rewrite it.
I.e. `my.example.com` will be rewritten to `my-example.local`.

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

Containers started with `docker-compose run` are ignored by
default. You can included them by setting the environment variable
`LDDDNS_IGNORE_DOCKER_COMPOSE_ONEOFF` to `false`.

The default configuration is the equivalent of setting:

```ini
[Service]
Environment=LDDDNS_HOSTNAME_LOOKUP=env:VIRTUAL_HOST,containerName
Environment=LDDDNS_IGNORE_DOCKER_COMPOSE_ONEOFF=true
```

## Install

For Pop!_OS, Ubuntu, Debian and the like, download the `.deb` package
file from the [latest
release](https://github.com/arnested/ldddns/releases/latest) and open
it or run:

```console
sudo dpkg -i ldddns_0.0.111_linux_amd64.deb
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
    Drop-In: /usr/lib/systemd/system/ldddns.service.d
             └─docker-version.conf
     Active: active (running) since Sat 2021-05-29 21:31:56 CEST; 1h 1min ago
       Docs: https://ldddns.arnested.dk
   Main PID: 133660 (ldddns)
     Status: "v0.0.111; HostnameLookup='env:VIRTUAL_HOST containerName';"
      Tasks: 14 (limit: 47843)
     Memory: 4.7M
     CGroup: /system.slice/ldddns.service
             └─133660 /usr/libexec/ldddns start

may 29 21:31:56 pop-os systemd[1]: Starting Local Docker Development DNS...
may 29 21:31:56 pop-os ldddns[133660]: Starting ldddns v0.0.111...
may 29 21:31:56 pop-os systemd[1]: Started Local Docker Development DNS.
may 29 21:31:56 pop-os ldddns[133660]: Rewrote hostname from "my.example.com" to "my-example.local"
may 29 21:31:56 pop-os ldddns[133660]: added address for "my-example.local" pointing to "172.18.0.2"
may 29 21:31:56 pop-os ldddns[133660]: added service "_https._tcp" pointing to "my-example.local"
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
