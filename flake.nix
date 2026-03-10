{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { nixpkgs, self, ... }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};
    in {
      packages.${system}.default = pkgs.buildGoModule {
        pname = "bugzilla-cli";
        version = "0.1.0";
        src = self;
        vendorHash = "sha256-pbA/AlBz3cQYRTMnQ/qBPcinYOKokrBLNhkbRTq54gE=";
        meta.mainProgram = "bugzilla-cli";
      };

      overlays.default = final: prev: {
        bugzilla-cli = self.packages.${prev.system}.default;
      };

      devShells.${system}.default = pkgs.mkShell {
        buildInputs = [ pkgs.go pkgs.gofumpt pkgs.golangci-lint ];
      };
    };
}
