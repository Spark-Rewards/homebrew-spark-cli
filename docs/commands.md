# spark-cli Command Reference

All commands support `-h` / `--help` for inline help. Commands are grouped below by category.

---

## Setup & Diagnostics

### `spark-cli setup`

First-time developer setup. Idempotent — safe to run multiple times.

```bash
spark-cli setup                # full setup (interactive)
spark-cli setup --skip-cache   # skip pre-warming the dev shell
```

**What it does (5 steps):**

1. Installs Nix via Determinate Systems if not present
2. Enables `nix-command flakes` in `~/.config/nix/nix.conf`
3. Patches `~/.zshrc` and `~/.zprofile` so `nix` is in PATH after login
4. Configures GitHub token in `~/.config/nix/nix.conf` and `~/.npmrc`
5. Pre-warms the dev shell (downloads Node 22, Java 17, AWS CLI, Gradle)

**Flags:**

| Flag | Description |
|------|-------------|
| `--skip-cache` | Skip step 5 — first `spark-cli dev` downloads instead |

---

### `spark-cli doctor`

Diagnoses your development environment. Run this first if anything seems broken.

```bash
spark-cli doctor
```

**Checks:**

```
[1/6] Nix installation         ✓  nix 2.33.x
[2/6] Nix flakes enabled       ✓  experimental-features = nix-command flakes
[3/6] GitHub token             ✓  access-tokens configured
[4/6] GitHub repo access       ✓  gh CLI authenticated
[5/6] Nix-tracked repos        ✓  14 repos (📁 local or 🌐 GitHub)
[6/6] npm GitHub Packages      ✓  @spark-rewards registry in ~/.npmrc
```

If any check fails, `doctor` tells you exactly how to fix it.

---

### `spark-cli init`

Initialise a new SparkRewards workspace (creates workspace config, installs Nix, runs setup).

```bash
spark-cli init
spark-cli init --skip-setup    # create workspace files only
```

> 💡 **Tip:** For new developers, `spark-cli setup` (from inside a cloned workspace) is simpler. Use `init` when creating a workspace from scratch.

---

## Development

### `spark-cli dev`

Enter the Nix dev shell. Provides a pinned, reproducible toolchain.

```bash
spark-cli dev
```

**Available in the shell:**

| Tool | Version | Purpose |
|------|---------|---------|
| `node` / `npm` / `tsc` | 22 | TypeScript services |
| `java` | 17 | Smithy codegen |
| `gradle` | 8 | InternalModel build |
| `aws` | 2 | Deployments |
| `git` | latest | Version control |
| `jq` | latest | JSON processing |

**When you enter:**

```
🔥 SparkRewards — Dev Shell
══════════════════════════════════════
  Node:    v22.14.0
  Java:    openjdk version "17.0.14"
  AWS CLI: aws-cli/2.24.12
  Gradle:  Gradle 8.12.1
  Git:     git version 2.47.2
══════════════════════════════════════

  Quick commands (your local repos):
    cd InternalAPILambda     && npm run watch     # TypeScript watch mode
    cd InternalWebsiteClient && npm start         # hot-reload website
```

> 💡 **Tip:** The quick commands section shows hints for repos you have cloned locally.

Exit with `Ctrl+D` or `exit`.

---

### `spark-cli build [package]`

Build a single SparkRewards package.

```bash
# By repo name (preferred):
spark-cli build InternalModel
spark-cli build InternalAPILambda
spark-cli build InternalWebsiteClient
spark-cli build InternalServiceCDK

# By Nix package name (equivalent):
spark-cli build internal-model
spark-cli build internal-api-lambda

# See all packages:
spark-cli build
```

**Package names — all three service chains:**

| Repo Name | Nix Package |
|-----------|------------|
| `InternalModel` | `internal-model` |
| `InternalAPILambda` | `internal-api-lambda` |
| `InternalWebsiteClient` | `internal-website-client` |
| `InternalServiceCDK` | `internal-service-cdk` |
| `BusinessModel` | `business-model` |
| `BusinessAPILambda` | `business-api-lambda` |
| `BusinessWebsite` | `business-website` |
| `BusinessServiceCDK` | `business-service-cdk` |
| `AppModel` | `app-model` |
| `AppAPILambda` | `app-api-lambda` |
| `AppServiceCDK` | `app-service-cdk` |
| `AuditorLambda` | `auditor-lambda` |
| `CorePipeline` | `core-pipeline` |
| `MobileApp` | `mobile-app` |

**Build groups** (build entire service chains at once):

```bash
spark-cli build internal-all     # all Internal packages
spark-cli build business-all     # all Business packages
spark-cli build app-all          # all App packages
spark-cli build all              # literally everything
```

**Output:**

```
🔥 Building InternalAPILambda (Lambda)

  [spark] 📁 Using local clone: InternalAPILambda
  ⚙  Building internal-api-lambda...
  [spark] Running tsc...
  [spark] TypeScript build complete ✓
  [spark] ✓ Lambda bundle → /nix/store/…-internal-api-lambda/lambda (52 JS files)

✓ Build complete  internal-api-lambda  (18s) → ./result

  Next:
    spark-cli cdk --profile pipeline diff    # preview changes
    spark-cli cdk --profile pipeline deploy  # deploy to beta
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--verbose` / `-v` | Show raw Nix output (useful for debugging build failures) |

---

### `spark-cli build-all`

Build all packages in dependency order.

```bash
spark-cli build-all
spark-cli build-all --verbose    # show raw Nix output
```

**Output:**

```
🔥 Building all packages

  📁 InternalModel              local [flame-develop]
  📁 InternalAPILambda          local [fix/my-change]
  🌐 InternalWebsiteClient      GitHub (pinned)
  🌐 InternalServiceCDK         GitHub (pinned)
  ...

  ⚙  Building internal-model...
  ⚙  Building internal-api-lambda...
  ...

─────────────────────────────────────────────────────
✓ All packages built (4m 12s) → ./result
```

If it fails:

```
✗ Build failed (45s)

  Tip: build individual packages to isolate the failure:
    spark-cli build InternalModel
    spark-cli build InternalAPILambda
    ...
```

---

### `spark-cli pr`

Push your current branch and create a GitHub PR. Detects the repo from your working directory.

```bash
cd ~/SparkRewards/InternalAPILambda
git checkout -b fix/my-change
# ... make changes ...
spark-cli pr
```

**What it does:**

1. Validates you're not on a protected branch (`main`, `flame-develop`, etc.)
2. `git push -u origin <branch>`
3. `gh pr create --base flame-develop` with the SparkRewards PR template pre-filled

**PR template (pre-filled):**

```markdown
## Description
fix my change

## Reason
<!-- Why is this change needed? -->

## Testing
- [ ] spark-cli build passes
- [ ] Tested locally
- [ ] Tested on beta (if applicable)
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--base`, `-b` | Base branch (default: `flame-develop`) |
| `--title`, `-t` | PR title (prompted interactively if not set) |
| `--draft` | Open as a draft PR |

**Examples:**

```bash
spark-cli pr                            # interactive
spark-cli pr --title "fix: null check in handler"
spark-cli pr --draft                    # WIP — not ready for review
spark-cli pr --base main                # PR directly to main (rare)
```

---

### `spark-cli status`

Shows which repos Nix will use locally vs fetch from GitHub.

```bash
spark-cli status
```

**Output:**

```
🔥 SparkRewards Nix Workspace Status
═══════════════════════════════════════════════════════
   Workspace:  /Users/you/SparkRewards
   GitHub Org: Spark-Rewards

   📦 Nix-tracked packages:
   REPO                           NIX PACKAGE               STATUS
   ─────────────────────────────  ────────────────────────  ─────────
   InternalModel                  internal-model            📁 local [flame-develop]
   InternalAPILambda              internal-api-lambda       📁 local [fix/my-change]
   InternalWebsiteClient          internal-website-client   🌐 GitHub (pinned)
   ...
```

📁 = using your local clone · 🌐 = will fetch from GitHub at build time

---

## Workspace Management

### `spark-cli use <repo>`

Clone a repo into the workspace.

```bash
spark-cli use InternalAPILambda
spark-cli use BusinessWebsite
spark-cli use other-org/SomeRepo    # external org
```

Once cloned, `spark-cli build` will use your local copy instead of GitHub.

---

### `spark-cli remove <repo>`

Remove a repo from the workspace (deletes the directory).

```bash
spark-cli remove InternalAPILambda
```

> 💡 **Tip:** After removing, builds continue to work — Nix will fetch the repo from GitHub at the pinned version.

---

### `spark-cli workspace`

Manage workspace configuration.

```bash
spark-cli workspace                           # show workspace info
spark-cli workspace create ~/MyWorkspace      # create a new workspace
spark-cli workspace configure --profile beta  # set default AWS profile
spark-cli workspace configure --list          # list available AWS SSO profiles
```

---

## Infrastructure

### `spark-cli cdk [cdk-args...]`

Run the AWS CDK CLI in the workspace CDK repo context. Injects workspace environment (GitHub token, AWS profile, etc.).

```bash
spark-cli cdk list                                      # list stacks
spark-cli cdk --profile pipeline diff                   # preview changes
spark-cli cdk --profile pipeline deploy InternalServiceStack
spark-cli cdk --profile pipeline deploy --all           # deploy everything
spark-cli cdk synth                                     # synthesize CloudFormation
```

**Profile flag:**

| Short name | AWS profile | Use for |
|-----------|-------------|---------|
| `pipeline` | `openclaw-pipeline` | All CDK deploys (has cross-account roles) |
| `beta` | `openclaw-beta` | Read-only / inspection |
| `prod` | `openclaw-prod` | Read-only only — **never deploy directly** |

> 💡 **Tip:** Always use `--profile pipeline` for deploys. The pipeline account has cross-account deploy roles for all environments.

> 🚨 **Never deploy to prod directly.** Prod is promoted via the `main` branch merge → CI/CD pipeline.

**Typical deploy workflow:**

```bash
spark-cli build-all                                      # 1. build
spark-cli cdk --profile pipeline list                    # 2. see stacks
spark-cli cdk --profile pipeline diff                    # 3. preview (always do this)
spark-cli cdk --profile pipeline deploy InternalServiceStack  # 4. deploy to beta
```

---

### `spark-cli run [command]`

Run any command with the workspace environment injected (GitHub token, AWS profile, etc.).

```bash
spark-cli run npm install
spark-cli run -- aws s3 ls s3://my-bucket
spark-cli run -- echo $GITHUB_TOKEN
```

Inside a Node/Gradle/Go repo, auto-maps to project scripts:

```bash
# Inside an npm project:
spark-cli run build       → npm run build
spark-cli run test        → npm test

# Inside a Gradle project:
spark-cli run build       → ./gradlew build
spark-cli run test        → ./gradlew test
```

---

## Quick Reference Card

```
Setup:
  spark-cli setup           # first-time onboarding
  spark-cli doctor          # health check

Dev:
  spark-cli dev             # enter Nix shell
  spark-cli build <repo>    # build one package
  spark-cli build-all       # build everything
  spark-cli pr              # push branch + create PR
  spark-cli status          # local vs GitHub

Workspace:
  spark-cli use <repo>      # clone a repo
  spark-cli remove <repo>   # remove a repo
  spark-cli workspace       # workspace info

Deploy:
  spark-cli cdk --profile pipeline diff    # preview
  spark-cli cdk --profile pipeline deploy  # deploy to beta
```
