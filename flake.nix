{
  description = "Local Docker Development DNS";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages = {
          ldddns = pkgs.buildGo126Module {
            pname = "ldddns";
            version = self.shortRev or self.dirtyShortRev or "dev";

            src = self;

            # Relax the go directive to match the Go version available in nixpkgs.
            prePatch = ''
              substituteInPlace go.mod --replace-fail "go 1.26.3" "go 1.26"
            '';

            vendorHash = "sha256-FI7kSIn1QNDcEqDECiGNVLZ5z7LWPxxunK6eKH3D46Y=";

            # Tests require /etc/protocols which is unavailable in the Nix sandbox.
            doCheck = false;

            env.CGO_ENABLED = 0;

            ldflags = [
              "-s"
              "-w"
              "-X main.version=${self.shortRev or self.dirtyShortRev or "dev"}"
            ];

            buildFlags = [ "-trimpath" ];

            postInstall = ''
              mv $out/bin/ldddns.arnested.dk $out/bin/ldddns
            '';

            meta = {
              description = "Local Docker Development DNS";
              homepage = "https://ldddns.arnested.dk";
              license = pkgs.lib.licenses.mit;
              maintainers = [ ];
              platforms = pkgs.lib.platforms.linux;
            };
          };

          default = self.packages.${system}.ldddns;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_26
            gopls
            goreleaser
          ];
        };
      }
    )
    // {
      nixosModules.default =
        {
          config,
          lib,
          pkgs,
          ...
        }:
        let
          cfg = config.services.ldddns;
        in
        {
          options.services.ldddns = {
            enable = lib.mkEnableOption "ldddns - Local Docker Development DNS";

            package = lib.mkOption {
              type = lib.types.package;
              default = self.packages.${pkgs.stdenv.hostPlatform.system}.ldddns;
              defaultText = lib.literalExpression "ldddns.packages.\${pkgs.stdenv.hostPlatform.system}.ldddns";
              description = "The ldddns package to use.";
            };

            hostnameLookup = lib.mkOption {
              type = lib.types.listOf lib.types.str;
              default = [
                "env:VIRTUAL_HOST"
                "containerName"
              ];
              description = "Methods for looking up hostnames for containers.";
            };

            ignoreDockerComposeOneoff = lib.mkOption {
              type = lib.types.bool;
              default = true;
              description = "Whether to ignore docker-compose oneoff containers.";
            };

            gops = lib.mkOption {
              type = lib.types.bool;
              default = false;
              description = "Whether to enable the Google gops diagnostics agent.";
            };
          };

          config = lib.mkIf cfg.enable {
            systemd.services.ldddns = {
              description = "Local Docker Development DNS";
              documentation = [ "https://ldddns.arnested.dk" ];
              bindsTo = [
                "docker.service"
                "avahi-daemon.service"
              ];
              after = [
                "docker.service"
                "avahi-daemon.service"
              ];
              wantedBy = [ "docker.service" ];

              environment = {
                LDDDNS_HOSTNAME_LOOKUP = lib.concatStringsSep "," cfg.hostnameLookup;
                LDDDNS_IGNORE_DOCKER_COMPOSE_ONEOFF =
                  if cfg.ignoreDockerComposeOneoff then "true" else "false";
                LDDDNS_GOPS = if cfg.gops then "true" else "false";
              };

              serviceConfig = {
                Type = "notify";
                ExecStart = "${cfg.package}/bin/ldddns start";
                Restart = "on-failure";
                SupplementaryGroups = [ "docker" ];
                CapabilityBoundingSet = "";
                DevicePolicy = "closed";
                IPAddressDeny = "any";
                LockPersonality = true;
                MemoryDenyWriteExecute = true;
                NoNewPrivileges = true;
                PrivateDevices = true;
                PrivateNetwork = true;
                PrivateUsers = true;
                ProtectClock = true;
                ProtectControlGroups = true;
                ProtectHome = true;
                ProtectHostname = true;
                ProtectKernelLogs = true;
                ProtectKernelModules = true;
                ProtectKernelTunables = true;
                RestrictAddressFamilies = [ "AF_UNIX" ];
                RestrictNamespaces = true;
                RestrictRealtime = true;
                SystemCallArchitectures = "native";
                SystemCallErrorNumber = "EPERM";
                SystemCallFilter = [ "@system-service" ];
                UMask = "0777";
              };
            };
          };
        };
    };
}
