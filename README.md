# spark-cli

Multi-repo workspace CLI for SparkRewards. Reproducible Nix builds — every developer gets the same toolchain, every build produces the same output.

## Install

```bash
brew tap Spark-Rewards/spark-cli
brew install spark-cli
```

## Quick Start

```bash
git clone git@github.com:Spark-Rewards/spark-workspace.git ~/SparkRewards
spark-cli setup        # install Nix, configure GitHub token, pre-warm cache
spark-cli dev          # enter dev shell (Node 22, Java 17, AWS CLI...)
spark-cli build-all    # build everything
```

## Documentation

| Guide | What's in it |
|-------|-------------|
| [Getting Started](docs/getting-started.md) | New developer onboarding, prerequisites, setup walkthrough, troubleshooting |
| [Daily Workflow](docs/daily-workflow.md) | Make changes, build, deploy to beta, create PRs, check CI |
| [Command Reference](docs/commands.md) | Every command with flags, examples, and real output |
| [How It Works](docs/how-it-works.md) | Nix internals, flake structure, build derivations, CI integration |

## Command Groups

```
Setup & Diagnostics:
  spark-cli setup       First-time onboarding (idempotent)
  spark-cli doctor      Health check — run if anything seems broken
  spark-cli init        Initialise a workspace from scratch

Development:
  spark-cli dev         Enter the Nix dev shell
  spark-cli build       Build one package
  spark-cli build-all   Build all packages
  spark-cli pr          Push branch + create PR with template
  spark-cli status      Local repos vs GitHub (pinned)

Workspace:
  spark-cli use         Clone a repo into the workspace
  spark-cli remove      Remove a repo
  spark-cli workspace   Workspace config (profile, env)

Infrastructure:
  spark-cli cdk         Run CDK (--profile pipeline for deploys)
  spark-cli run         Run any command with workspace env injected
```

Run `spark-cli <command> -h` for detailed help on any command.

## How It Works (Short Version)

SparkRewards uses a [Brazil-like](https://blog.acolyer.org/2020/03/13/build-and-dev-tools-at-amazon/) sparse checkout model:

- Clone only the repos you're actively editing
- Everything else is fetched from GitHub at the pinned `flake.lock` version
- `spark-cli dev` gives you an identical toolchain on every machine
- `spark-cli build` = `nix build --impure .#<package>` with output filtering

See [docs/how-it-works.md](docs/how-it-works.md) for the full explanation.
