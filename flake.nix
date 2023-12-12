{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils/v1.0.0";
  };

  description = "gitlab-ci-crawler, building a dependency graph for CI includes";

  outputs = { self, nixpkgs, flake-utils }:
  flake-utils.lib.eachSystem [ "x86_64-linux" "i686-linux" ] (system:
    let
      pkgs = nixpkgs.legacyPackages.${system};
      build = pkgs.buildGoModule {
        pname = "gitalb-ci-crawler";
        version = "v0.13.15";
        modSha256 = pkgs.lib.fakeSha256;
        vendorSha256 = null;
        src = ./.;

        meta = {
          description = "gitlab-ci-crawler, building a dependency graph for CI includes";
          homepage = "https://github.com/catouc/gitlab-ci-crawler";
          license = pkgs.lib.licenses.mit;
          maintainers = [ "catouc" ];
          platforms = pkgs.lib.platforms.linux ++ pkgs.lib.platforms.darwin;
        };
      };
  in
    rec {
      packages = {
        gitlab-ci-crawler = build;
        default = build;
      };

      hydraJobs = {inherit packages;};

      devShells = {
        default = pkgs.mkShell {
          buildInputs = [
            pkgs.delve
            pkgs.docker
            pkgs.go_1_21
            pkgs.gopls
            pkgs.gotools
          ];
          shellHook = ''
            export NEO4J_USERNAME='neo4j'
            export NEO4J_PASSWORD='neo4j'
	    export CGO_ENABLED=0
          '';
        };
      };
    }
  );
}
