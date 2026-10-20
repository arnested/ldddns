# Local Docker Development DNS

A systemd service that will monitor your Docker host and provide
DNS names for the containers.

The service answers for the domain names with its own multicast DNS
responder (a.k.a. mDNS, zeroconf, Bonjour). It answers only on the
Docker networks of the containers, so the names do not reach your LAN.

A limitation of this is that domain names can only be on the `.local`
TLD. A benefit is that you don't have to change your DNS server or
configure stuff in `/etc/resolv` or similar.

If the containers also have exposed ports (and the ports can be looked
up in `/etc/services`) the service will also broadcast the
service/domain for service discovery. I.e., `_https._tcp.` for
<https://my-example.local> (a.k.a. DNS-SD). Only _one_ domain name can
be broadcast per service per container.

Per default domain names will be generated from the `VIRTUAL_HOST`
environment variable is present (several hostnames can be separated by
space or comma) and from the container name.

If a hostname is not on the `.local` TLD, the service replaces the
public suffix with `.local`. The service keeps the dots, and it also
publishes a flat name with only one level below `.local`. For
example, `my.example.com` gives both `my.example.local` and
`my-example.local`.

The default configuration of nss-mdns (`mdns4_minimal` in
`/etc/nsswitch.conf`) resolves only names with one level below
`.local`. The flat names work everywhere, so the DNS-SD services
point to the flat name. To resolve names like
`my.example.local` too, use systemd-resolved with `MulticastDNS=yes`.
Or change `mdns4_minimal` to `mdns4` in `/etc/nsswitch.conf`, and
create `/etc/mdns.allow` with the two lines `.local.` and `.local`.

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

## Requirements

The host needs an mDNS resolver, because `ldddns` only answers for
the names. Use one of these:

* Avahi with nss-mdns (`avahi-daemon` and `libnss-mdns`). Most
  desktop distributions install these by default.
* systemd-resolved with `MulticastDNS=yes`.

`ldddns` shares UDP port 5353 with the resolver.

## Install

For Pop!_OS, Ubuntu, Debian and the like, download the `.deb` package
file from the [latest
release](https://github.com/arnested/ldddns/releases/latest) and open
it or run:

```console
sudo dpkg -i ldddns_1.0.0_linux_amd64.deb
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
     Active: active (running) since Mon 2022-01-03 09:13:14 CET; 5 days ago
       Docs: https://ldddns.arnested.dk
   Main PID: 5414 (ldddns)
     Status: "v1.0.13; {"HostnameLookup":["env:VIRTUAL_HOST","containerName"],"IgnoreDockerComposeOneoff":true}"
      Tasks: 14 (limit: 47870)
     Memory: 13.7M
        CPU: 6.243s
     CGroup: /system.slice/ldddns.service
             └─5414 /usr/libexec/ldddns start

jan 07 12:46:11 pop-os systemd[1]: Starting Local Docker Development DNS...
jan 07 12:46:11 pop-os ldddns[5414]: Starting ldddns v1.0.13...
jan 07 12:46:11 pop-os systemd[1]: Started Local Docker Development DNS.
jan 07 12:46:11 pop-os ldddns[5414]: Rewrote hostname from "my.example.com" to "my-example.local"
jan 07 12:46:12 pop-os ldddns[5414]: mDNS name my-example.local. of container 3f0c… is live
jan 07 12:46:12 pop-os ldddns[5414]: mDNS name my-example._https._tcp.local. of container 3f0c… is live
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
