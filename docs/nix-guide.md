# Nix Developer Guide

## What is Nix?

Nix is a package manager that gives every developer the **exact same tooling** — same Node version, same JDK, same Gradle, same everything. No more "works on my machine."

Instead of each dev installing tools manually (and ending up with different versions), Nix reads a `flake.nix` file and sets up a reproducible environment.

## Why We Use It

- **Reproducible builds**: CI and your laptop use identical tool versions
- **No global installs**: Nix tools don't pollute your system — they live in `/nix/store`
- **One command setup**: New devs run one command and have everything
- **Pinned versions**: `flake.lock` pins exact versions. No surprise upgrades

## Quick Start

### 1. Install Nix

```bash
# macOS / Linux
curl --proto '=https' --tlsv1.2 -sSf -L https://install.determinate.systems/nix | sh -s -- install
```

Restart your terminal after installation.

### 2. Enter the Dev Environment

```bash
cd ~/SparkRewards
nix develop --impure
```

This drops you into a shell with Node 22, JDK 17, Gradle, AWS CLI, and everything else pre-configured. First run downloads dependencies (~2-5 min). Subsequent runs are instant.

### 3. Verify

```bash
node --version    # v22.x.x
java --version    # openjdk 17.x
gradle --version  # 8.x
aws --version     # aws-cli/2.x
```

## How It Works

### flake.nix

Each repo has a `flake.nix` that declares what tools it needs:

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };
  outputs = { nixpkgs, ... }:
    let pkgs = nixpkgs.legacyPackages.x86_64-linux;
    in {
      devShells.default = pkgs.mkShell {
        buildInputs = [
          pkgs.nodejs_22
          pkgs.jdk17
          pkgs.gradle
          pkgs.awscli2
        ];
      };
    };
}
```

### flake.lock

Auto-generated file that pins exact versions. Commit this — it's what makes builds reproducible.

### Workspace Flake

`~/SparkRewards/flake.nix` is the **workspace-level flake** that covers all 14 repos. When you `nix develop` from `~/SparkRewards`, you get everything needed for any repo.

## Daily Workflow

### Using spark-cli (recommended)

`spark-cli` wraps commands in Nix automatically:

```bash
spark-cli run build          # Build current repo
spark-cli run test           # Run tests
spark-cli cdk deploy "..."   # CDK deploy (wraps in nix develop)
spark-cli doctor             # Verify all 14 repos
```

### Using Nix directly

```bash
# Enter the dev shell
cd ~/SparkRewards
nix develop --impure

# Now run commands normally
cd InternalAPILambda
npm install
npm run build
npm test

# Or one-liner without entering the shell
nix develop --impure --command bash -c 'npm ci && npm run build'
```

### The `--impure` Flag

We use `--impure` because our flakes reference environment variables (like `$HOME`) and system paths. This is normal for dev environments.

## CI / Pipelines

### GitHub Actions (PR Checks)

CI uses `DeterminateSystems/nix-installer-action` to install Nix, then runs builds inside `nix develop`:

```yaml
steps:
  - uses: actions/checkout@v4
  - uses: DeterminateSystems/nix-installer-action@main
  - name: Build and test
    run: |
      nix develop --impure --command bash -c '
        npm ci
        npm run build
        npm test
      '
    env:
      GITHUB_TOKEN: ${{ secrets.ORG_PAT }}
```

### CodeBuild Pipelines (CDK Deploy)

Pipelines use a custom Docker image with Nix pre-installed (`spark/codebuild-nix:latest` in ECR). The buildspec wraps synth in `nix develop`:

```yaml
build:
  commands:
    - nix develop --impure --command bash -c 'npm i && npx cdk synth'
```

## Updating Dependencies

### Update all Nix packages

```bash
nix flake update
# Then commit flake.lock
```

### Update a specific input

```bash
nix flake lock --update-input nixpkgs
```

## Troubleshooting

### "command not found" after installing Nix

Restart your terminal. If still broken:
```bash
export PATH="/nix/var/nix/profiles/default/bin:$PATH"
```

### Slow first run

Normal. Nix downloads and caches packages on first use. Subsequent runs are instant.

### "error: experimental feature 'nix-command' is disabled"

Your Nix installation might be old. Reinstall with Determinate Systems installer (above).

### Build fails with "exit 127" in CodeBuild

Don't cache `/nix/**/*` in CodeBuild — it overwrites the Docker image's Nix installation with an empty cache directory. Only cache `node_modules/**/*`.

## Key Files

| File | Purpose |
|------|---------|
| `~/SparkRewards/flake.nix` | Workspace-level dev environment |
| `~/SparkRewards/flake.lock` | Pinned package versions |
| `<repo>/flake.nix` | Per-repo dev environment |
| `<repo>/.npmrc` | npm registry config for @spark-rewards packages |

## FAQ

**Q: Do I need Nix installed globally?**
A: Yes, Nix itself is global (`/nix/`), but the packages it manages are isolated and don't affect your system.

**Q: Can I still use my system Node/Java?**
A: Inside `nix develop`, Nix's versions take priority. Outside, your system tools work normally.

**Q: What if I need a package that's not in the flake?**
A: Add it to `flake.nix` and run `nix develop` again. Ask in Slack if unsure.

**Q: Does Nix work on Windows?**
A: Only via WSL2. Native Windows is not supported.
