// Package assets embeds workspace template files into the spark-cli binary.
package assets

import "embed"

//go:embed flake.nix flake.lock setup.sh gitignore README.md buildspec-nix.yml
var FS embed.FS
