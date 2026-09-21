{
  description = "Palomas del Gobierno — sitio web de la banda (Go + SQLite)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      devShells = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              sqlite
              w3m
              lynx
              curl
            ];
            shellHook = ''
              echo "=============================================="
              echo " PALOMAS DEL GOBIERNO — shell de desarrollo"
              echo " go:      $(go version 2>/dev/null || echo n/a)"
              echo " uso:     go run . [-addr :8080]"
              echo " w3m:     w3m http://localhost:8080  (lectura)"
              echo "=============================================="
            '';
          };
        });

      packages = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        rec {
          default = pkgs.buildGoModule {
            pname = "palomas";
            version = "0.1.0";
            src = ./.;
            vendorHash = null;
            meta = {
              description = "Sitio web de la banda Palomas del Gobierno";
              mainProgram = "palomas";
            };
          };
        });
    };
}
