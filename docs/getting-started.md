# Getting Started with SparkRewards Development

Welcome. This guide gets you from zero to building in under 10 minutes.

---

## Prerequisites

| Requirement | Version | Install |
|-------------|---------|---------|
| macOS | 13+ (Intel or Apple Silicon) | — |
| Homebrew | latest | `curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh \| bash` |
| Git | any | `xcode-select --install` |
| GitHub account | — | [github.com](https://github.com) |
| GitHub CLI | any | `brew install gh && gh auth login` |

> 💡 **Tip:** You don't need Node.js, Java, or any other language toolchain installed globally. `spark-cli` installs everything for you via Nix.

---

## Step 1 — Install spark-cli

```bash
brew tap Spark-Rewards/spark-cli
brew install spark-cli
```

Verify:

```bash
spark-cli --version
# spark-cli v0.3.x (...)
```

---

## Step 2 — Clone the Workspace

```bash
git clone git@github.com:Spark-Rewards/spark-workspace.git ~/SparkRewards
```

> 💡 **Tip:** `~/SparkRewards` is the expected workspace path. All repos go here as subdirectories. If you prefer a different location, set `SPARK_WORKSPACE=/your/path` in your shell environment.

---

## Step 3 — Run Setup

```bash
spark-cli setup
```

This is idempotent — safe to run multiple times. It does:

```
Step 1/5: Nix installation
  → Installs Nix via Determinate Systems (2–5 min, asks for sudo once)
  → Skipped if Nix is already installed

Step 2/5: Nix flakes
  → Enables 'nix-command flakes' in ~/.config/nix/nix.conf
  → Skipped if already enabled

Step 3/5: Shell PATH
  → Adds nix-daemon.sh to ~/.zshrc and ~/.zprofile
  → Ensures 'nix' is in PATH after a fresh terminal login
  → Skipped if already patched

Step 4/5: GitHub token
  → Reads from 'gh auth token' automatically (if gh CLI is logged in)
  → Or prompts you to paste a GitHub personal access token
  → Writes to ~/.config/nix/nix.conf (for private repo cloning)
  → Writes to ~/.npmrc (for @spark-rewards npm packages)
  → Skipped if already configured

Step 5/5: Pre-warming dev shell
  → Downloads Node.js 22, Java 17, AWS CLI, Gradle, etc. into the Nix store
  → Takes 3–10 minutes on first run, then instant forever
  → Skip with: spark-cli setup --skip-cache
```

When done:

```
╔══════════════════════════════════════════════════╗
║   ✅ Setup Complete!                              ║
╚══════════════════════════════════════════════════╝
```

---

## Step 4 — Clone Repos to Work On

You don't need to clone every repo. Clone only what you're actively editing:

```bash
spark-cli use InternalAPILambda     # Lambda service
spark-cli use InternalWebsiteClient # Frontend website
```

Everything else is fetched from GitHub automatically at build time.

See what's cloned locally vs remote:

```bash
spark-cli status
```

---

## Step 5 — Enter the Dev Shell

```bash
spark-cli dev
```

You'll see:

```
🔥 SparkRewards — Dev Shell
══════════════════════════════════════
  Node:    v22.x.x
  Java:    openjdk version "17.x.x"
  AWS CLI: aws-cli/2.x.x
  Gradle:  Gradle 8.x
  Git:     git version 2.x.x
══════════════════════════════════════
```

You're now in a shell with a pinned, reproducible toolchain. `node`, `npm`, `tsc`, `java`, `gradle`, `aws` are all available.

> 💡 **Tip:** This shell is identical to CI. If it builds here, it builds in CI.

Exit with `Ctrl+D` or `exit`.

---

## Step 6 — Build

```bash
spark-cli build InternalAPILambda
```

Or build everything:

```bash
spark-cli build-all
```

First build downloads any repos you haven't cloned (from GitHub) and compiles them. Subsequent builds are cached by Nix.

---

## Verify Your Setup

```bash
spark-cli doctor
```

All 6 checks should be green:

```
[1/6] Nix installation         ✓
[2/6] Nix flakes enabled       ✓
[3/6] GitHub token             ✓
[4/6] GitHub repo access       ✓
[5/6] Nix-tracked repos        ✓  (14 repos tracked)
[6/6] npm GitHub Packages      ✓
```

---

## Troubleshooting

### `nix: command not found`

Your shell hasn't picked up the Nix PATH yet. Fix:

```bash
# Temporary (this session):
. '/nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh'

# Permanent (run setup — patches ~/.zshrc and ~/.zprofile):
spark-cli setup
```

Then open a new terminal.

### `spark-cli: command not found`

```bash
brew tap Spark-Rewards/spark-cli
brew install spark-cli
```

### `error: access to GitHub denied` or `403 Forbidden`

Your GitHub token isn't configured or has expired:

```bash
gh auth login           # re-authenticate with GitHub CLI
spark-cli setup         # re-runs token configuration step
```

### `@spark-rewards/internal-sdk: Not found` during npm install

The `@spark-rewards` npm packages are on GitHub Packages and require authentication. Run:

```bash
spark-cli setup         # configures ~/.npmrc with GitHub Packages auth
```

### Build is very slow (first time)

Normal. Nix is downloading the full toolchain (Node 22 + Java 17 + AWS CLI + Gradle). This is a one-time cost — everything is cached in the Nix store afterward.

To pre-warm:

```bash
spark-cli setup         # step 5 pre-warms the cache
```

### Something else broken?

```bash
spark-cli doctor        # diagnose all checks
spark-cli setup         # re-run setup (idempotent — safe to re-run)
```

---

## Next Steps

- [Daily Workflow →](daily-workflow.md) — how to make changes, build, deploy, and create PRs
- [Command Reference →](commands.md) — all spark-cli commands with examples
- [How It Works →](how-it-works.md) — understand the Nix build system
