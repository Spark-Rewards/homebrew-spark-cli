# spark-cli

Multi-repo workspace CLI for SparkRewards development. Includes a Brazil-like Nix build system for reproducible builds.

**New to spark-cli?** See **[docs/GETTING_STARTED.md](docs/GETTING_STARTED.md)** for a full guide.

## Install

```bash
brew install spark-rewards/spark-cli/spark-cli
```

## Quick Start — New Developer

```bash
# 1. Clone the workspace
git clone git@github.com:Spark-Rewards/spark-workspace.git ~/SparkRewards
cd ~/SparkRewards

# 2. Run one-time setup
spark-cli setup
# → Installs Nix, configures GitHub token, pre-warms dev environment

# 3. Enter the dev shell (Node 22, Java 17, AWS CLI — guaranteed consistent)
spark-cli dev

# 4. Clone just what you're working on
spark-cli use InternalAPILambda

# 5. Build — everything else fetched from GitHub automatically
spark-cli build-all
```

## Commands

### Nix Build System (reproducible builds)

| Command | Description |
|---------|-------------|
| `spark-cli setup` | First-time setup: install Nix, configure GitHub token, pre-warm dev shell |
| `spark-cli dev` | Enter the Nix dev shell (Node 22, Java 17, AWS CLI, Gradle) |
| `spark-cli build <package>` | Build a specific package (also accepts repo names) |
| `spark-cli build-all` | Build all packages in dependency order |
| `spark-cli status` | Show which repos are local 📁 vs fetched from GitHub 🌐 |
| `spark-cli doctor` | Check Nix environment health (6 checks: Nix, flakes, token, access, repos, npm) |

**Package names:**

| Repo Name | Nix Package |
|-----------|------------|
| `InternalModel` | `internal-model` |
| `InternalAPILambda` | `internal-api-lambda` |
| `InternalWebsiteClient` | `internal-website-client` |
| `InternalServiceCDK` | `internal-service-cdk` |

Both forms are accepted: `spark-cli build InternalModel` = `spark-cli build internal-model`

### Workspace Management

| Command | Description |
|---------|-------------|
| `spark-cli use <repo>` | Clone a repo into the workspace |
| `spark-cli workspace` | Show workspace info (repos, AWS profile, branches) |
| `spark-cli workspace create <path>` | Create a new workspace |
| `spark-cli workspace configure --profile <name>` | Set default AWS profile |
| `spark-cli run [script]` | Run a script with workspace env injected |
| `spark-cli cdk [args...]` | Run AWS CDK CLI in the workspace CDK repo |
| `spark-cli remove <repo>` | Remove a repo from the workspace manifest |
| `spark-cli sync` | Sync all repos and refresh `.env` from AWS |

## How the Nix Build System Works

Inspired by Amazon's Brazil build system:

- **Clone only what you're working on.** Other repos are fetched from GitHub automatically at the pinned `flake.lock` version.
- **`spark-cli dev` gives a guaranteed toolchain.** Node 22, Java 17, AWS CLI — identical on every machine.
- **Reproducible builds.** Same input always produces the same output, local = CI.

```
InternalModel (Gradle/Smithy)
  └── generates TypeScript SDK
       ├── InternalAPILambda (TypeScript Lambda)
       ├── InternalWebsiteClient (TypeScript + Vite)
       └── InternalServiceCDK (AWS CDK)
```

## Setup Details

`spark-cli setup` is idempotent — safe to run multiple times:

1. Installs Nix via [Determinate Systems installer](https://install.determinate.systems/)
2. Enables flakes in `~/.config/nix/nix.conf`
3. Configures GitHub token (auto-reads from `gh auth token` or prompts)
   - Writes to `~/.config/nix/nix.conf` for private repo access
   - Writes to `~/.npmrc` for `@spark-rewards` npm packages
4. Pre-warms the dev shell (downloads toolchain packages, ~3–10 min first time)

```bash
spark-cli setup             # full setup
spark-cli setup --skip-cache  # skip pre-warming (faster; first 'spark-cli dev' downloads instead)
```

## Troubleshooting

```bash
spark-cli doctor    # diagnose any issues
spark-cli setup     # re-run setup to fix automatically
```

Run `spark-cli <command> --help` for detailed help on any command.
