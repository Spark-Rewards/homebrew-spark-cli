# 🔥 SparkRewards — Nix Build System

Reproducible, Brazil-like builds for the SparkRewards monorepo. Every developer gets the same toolchain. Every build produces the same output.

---

## Prerequisites

- **macOS** (Intel or Apple Silicon) — Linux also supported
- **git** — `xcode-select --install` on macOS if missing
- **GitHub access** — SSH key or personal access token with `read:packages` + `repo` scopes
  - Get a token: https://github.com/settings/tokens/new
  - Or use GitHub CLI: `brew install gh && gh auth login`

---

## Quick Start

```bash
# 1. Install spark-cli (if not already installed)
brew install spark-rewards/spark-cli/spark-cli

# 2. Clone this workspace
git clone git@github.com:Spark-Rewards/spark-workspace.git ~/SparkRewards
cd ~/SparkRewards

# 3. Run one-time setup (installs Nix, configures GitHub token, pre-warms shell)
spark-cli setup

# 4. Enter the dev shell — guaranteed Node 22, Java 17, AWS CLI
spark-cli dev

# 5. Clone just what you're working on
spark-cli use InternalAPILambda

# 6. Build — everything else fetched from GitHub automatically
spark-cli build-all
```

That's it. No `brew install node`, no version managers, no manual `npm install`.

---

## spark-cli Command Reference

The Nix build system is integrated into `spark-cli`. No separate tool needed.

### Nix Build Commands

| Command | Description |
|---------|-------------|
| `spark-cli setup` | First-time setup (idempotent, safe to re-run) |
| `spark-cli dev` | Enter the dev shell with pinned toolchain |
| `spark-cli build <package>` | Build a specific package |
| `spark-cli build-all` | Build all packages in dependency order |
| `spark-cli use <repo>` | Clone a repo into the workspace |
| `spark-cli status` | Show local 📁 vs GitHub 🌐 for all repos |
| `spark-cli doctor` | Check environment health — run this if anything seems broken |

### Package Names

Both repo names and Nix package names are accepted:

| Repo Name | Nix Package Name |
|-----------|-----------------|
| `InternalModel` | `internal-model` |
| `InternalAPILambda` | `internal-api-lambda` |
| `InternalWebsiteClient` | `internal-website-client` |
| `InternalServiceCDK` | `internal-service-cdk` |

```bash
# These are equivalent:
spark-cli build InternalModel
spark-cli build internal-model
```

---

## How It Works — Brazil-like Sparse Checkout

This system is inspired by Amazon's [Brazil build system](https://blog.acolyer.org/2020/03/13/build-and-dev-tools-at-amazon/).

**Core idea:** Clone only the repos you're actively editing. Everything else is fetched from GitHub automatically at build time.

```
You have locally:          InternalAPILambda (editing)
Nix fetches from GitHub:   InternalModel, InternalWebsiteClient, InternalServiceCDK
```

Nix resolves this at evaluation time using `--impure` mode (reads your local filesystem). If a repo folder exists in `~/SparkRewards/`, it uses your local copy. If not, it pulls the pinned commit from `flake.lock`.

### Dependency Graph

```
InternalModel (Gradle/Smithy)
  └── generates TypeScript SDK (@spark-rewards/internal-sdk)
       ├── InternalAPILambda (TypeScript Lambda)
       ├── InternalWebsiteClient (TypeScript + Vite)
       └── InternalServiceCDK (AWS CDK)
            └── CorePipeline
```

---

## Dev Shell

```bash
spark-nix dev
```

Inside the dev shell, you have:

| Tool | Version | Purpose |
|------|---------|---------|
| Node.js | 22 | TypeScript services |
| Java | 17 | Gradle / Smithy codegen |
| AWS CLI | 2 | Deployments |
| Gradle | 8 | InternalModel build |
| git | latest | Version control |
| jq | latest | JSON processing |
| TypeScript | latest | `tsc` global |

The shell is guaranteed to be identical on every machine that runs `nix develop --impure`. No more "works on my machine."

---

## Build Outputs

After `spark-nix build-all`, the `./result` symlink points to the Nix store:

```
result/
├── lambda/          # InternalAPILambda compiled output (deploy to Lambda)
├── dist/            # InternalWebsiteClient Vite bundle (deploy to S3)
├── cdk.json         # InternalServiceCDK CDK config
├── sdk/             # InternalModel TypeScript SDK source
├── client/          # InternalModel TypeScript client source
└── openapi/         # InternalModel OpenAPI spec
```

---

## CI/CD

CodeBuild integration is configured in `buildspec-nix.yml`. It uses the same `flake.nix` as local development, ensuring identical builds.

**Required CodeBuild environment variables:**

| Variable | Source | Purpose |
|----------|--------|---------|
| `GITHUB_TOKEN` | Secrets Manager: `sparkrewards/github-token:token` | Private repo access + npm auth |

See `buildspec-nix.yml` for full CI configuration and tuning notes.

---

## FAQ

### "Nix not found" or setup errors

Run setup first:
```bash
spark-cli setup
```

If spark-cli isn't installed yet, install via Homebrew:
```bash
brew install spark-rewards/spark-cli/spark-cli
```

### "error: flake input X is not a flake" or authentication errors

Your GitHub token may have expired or not be configured. Run:
```bash
spark-nix doctor     # diagnose the issue
spark-nix setup      # re-run setup to fix
```

Or manually update the token:
```bash
echo "access-tokens = github.com=$(gh auth token)" >> ~/.config/nix/nix.conf
```

### "npm install failed" or "@spark-rewards/internal-sdk not found"

The `@spark-rewards` npm packages are on the GitHub Packages registry and require auth. Ensure your `GITHUB_TOKEN` is set:

```bash
export GITHUB_TOKEN=$(gh auth token)
nix build --impure .#internal-api-lambda
```

Or run `spark-nix setup` to configure this globally.

### "Build is slow — 15+ minutes"

First builds download Maven dependencies for Gradle (one-time). Subsequent builds are ~2-5 minutes. To speed things up significantly, set up a [Cachix binary cache](https://www.cachix.org/) — see the `buildspec-nix.yml` comments.

### "I changed code but the build isn't picking it up"

Nix uses content-addressed hashing. If you edited files in a locally-cloned repo, the build should pick them up automatically (the `localOrGitHub` function re-hashes on each evaluation).

If it still seems stale:
```bash
spark-nix clean          # remove old derivations
spark-nix build-all      # rebuild
```

### "How do I update to the latest version of a dependency?"

```bash
spark-nix update         # updates flake.lock to latest GitHub commits
git add flake.lock
git commit -m "chore: update nix flake.lock"
```

### "I want to work on a repo not in the Nix system yet"

Use spark-cli for other repos. The Nix system currently covers the Internal service chain (the 4 repos above). Extending to Business/App chains is on the roadmap.

---

## Known Limitations

1. **Gradle builds use network access** (`__noChroot = true`) — Gradle downloads Maven deps at build time. This means InternalModel builds require internet access. A fully hermetic solution (pre-fetched Maven cache) is on the roadmap.

2. **npm builds require GITHUB_TOKEN** — Fresh `npm install` for `@spark-rewards` packages needs a GitHub token. This is automatically handled when `GITHUB_TOKEN` is set in the environment (which `setup.sh` configures).

3. **macOS only for now** — The flake targets `aarch64-darwin`. Adding `x86_64-linux` support for CI is straightforward but not done yet.

4. **No Cachix binary cache yet** — First-run builds download everything from scratch. Setup time is 10-25 minutes without a cache.

5. **Nix sandbox disabled on macOS** — macOS doesn't support Linux-style sandboxing; builds have access to the network. This is normal for macOS Nix usage.

---

## Files

```
~/SparkRewards/
├── flake.nix              ← Main Nix build definition
├── flake.lock             ← Pinned dependency versions (COMMIT THIS)
├── setup.sh               ← First-time setup script
├── spark-nix.sh           ← spark-nix CLI (symlinked to /usr/local/bin)
├── buildspec-nix.yml      ← AWS CodeBuild CI config
├── README.md              ← This file
├── .gitignore             ← Ignores result, .direnv
├── InternalModel/         ← Cloned repo (if working on it locally)
├── InternalAPILambda/     ← Cloned repo (if working on it locally)
├── InternalWebsiteClient/ ← Cloned repo (if working on it locally)
├── InternalServiceCDK/    ← Cloned repo (if working on it locally)
└── result -> /nix/store/… ← Build output symlink (after nix build)
```
