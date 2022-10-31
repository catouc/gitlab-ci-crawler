{
  inputs = {
    flake-utils.url = "github:numtide/flake-utils/v1.0.0";
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  description = "gitlab-ci-crawler, building a dependency graph for CI includes";

  outputs = { self, nixpkgs, flake-utils }:
  flake-utils.lib.eachDefaultSystem (system:
  let
    pkgs = nixpkgs.legacyPackages.${system};
  in
    rec {
      packages = flake-utils.lib.flattenTree {
        gitlabCICrawler = pkgs.buildGoModule {
          pname = "gitalb-ci-crawler";
          version = "v0.13.5";
          modSha256 = pkgs.lib.fakeSha256;
          vendorSha256 = null;
          src = ./.;

          meta = {
            description = "gitlab-ci-crawler, building a dependency graph for CI includes";
            homepage = "https://github.com/catouc/gitlab-ci-crawler";
            license = pkgs.lib.licenses.mit;
            maintainers = [ "catouc" ];
            platforms = pkgs.lib.platforms.linux;
          };
        };
      };

      defaultPackage = packages.gitlabCICrawler;
      defaultApp = packages.gitlabCICrawler;

      devShell = pkgs.mkShell {
        buildInputs = [
          pkgs.docker
          pkgs.gcc
          pkgs.go
        ];
        shellHook = ''
          export NEO4J_USERNAME='neo4j'
          export NEO4J_PASSWORD='neo4j'
        '';
      };
    }
  );
}
