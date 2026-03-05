# Daily Development Workflow

The everyday loop: make a change → build → test → deploy to beta → open PR → merge.

---

## Starting Your Day

```bash
spark-cli dev
```

That's it. The dev shell gives you everything: `node`, `npm`, `tsc`, `java`, `gradle`, `aws`, `git`.

Check what you have cloned locally:

```bash
spark-cli status
```

---

## Making Changes

### Branch from `flame-develop`

```bash
cd ~/SparkRewards/InternalAPILambda   # or whichever repo you're editing
git fetch origin
git checkout flame-develop
git pull --rebase origin flame-develop
git checkout -b fix/my-change         # or feat/my-feature, chore/cleanup, etc.
```

> 💡 **Tip:** Never commit directly to `main` or `flame-develop`. Always work on a feature branch.

### Edit code

Use your editor normally. Nix doesn't interfere with how you edit files — it only kicks in when you build.

---

## Hot-Reload During Development

Some repos support hot-reload so you get instant feedback while editing.

**Inside the dev shell** (`spark-cli dev`):

```bash
# TypeScript Lambda — watch mode (recompiles on save):
cd ~/SparkRewards/InternalAPILambda && npm run watch

# Website — Vite dev server with hot-reload:
cd ~/SparkRewards/InternalWebsiteClient && npm start
cd ~/SparkRewards/BusinessWebsite && npm start

# Smithy model — rebuild SDK on changes:
cd ~/SparkRewards/InternalModel && ./gradlew build --continuous
```

> 💡 **Tip:** Hot-reload works directly in the repo directory — you don't need `spark-cli build` for every save. Use `spark-cli build` for a clean reproducible build before deploying.

---

## Building

### Build the repo you changed

```bash
spark-cli build InternalAPILambda
```

### Build everything (safe to run anytime)

```bash
spark-cli build-all
```

### Isolate a build failure

If `build-all` fails, narrow it down by building the chain one step at a time:

```bash
spark-cli build InternalModel           # step 1: Smithy SDK
spark-cli build InternalAPILambda       # step 2: Lambda
spark-cli build InternalWebsiteClient   # step 3: Website
spark-cli build InternalServiceCDK      # step 4: CDK (uses Lambda + Website)
```

The first failure shows you where to look.

### Full Nix output (for debugging)

```bash
spark-cli build InternalAPILambda --verbose
```

---

## Common Change Patterns

### Change a Lambda handler

```bash
cd ~/SparkRewards/InternalAPILambda
# edit src/handler.ts or src/operations/...
spark-cli build InternalAPILambda
spark-cli cdk --profile pipeline deploy InternalServiceStack
```

### Update the Smithy model (adds/changes an API operation)

The model generates the TypeScript SDK, which both Lambda and Website depend on.

```bash
cd ~/SparkRewards/InternalModel
# edit smithy/model/...

# Build the chain in order:
spark-cli build InternalModel           # regenerates TypeScript SDK
spark-cli build InternalAPILambda       # recompiles with new SDK
spark-cli build InternalWebsiteClient   # recompiles with new client SDK
spark-cli build InternalServiceCDK      # repackages CDK

# Deploy:
spark-cli cdk --profile pipeline deploy InternalServiceStack
```

> 💡 **Tip:** You don't need to run all four builds separately. `spark-cli build internal-all` builds the full chain in order.

### Update the CDK infrastructure (change Lambda config, add resource, etc.)

```bash
cd ~/SparkRewards/InternalServiceCDK
# edit lib/...

spark-cli build InternalServiceCDK
spark-cli cdk --profile pipeline diff           # always preview first
spark-cli cdk --profile pipeline deploy InternalServiceStack
```

### Change a website component

```bash
cd ~/SparkRewards/InternalWebsiteClient
# edit src/...

# Hot-reload during development:
npm start    # (inside spark-cli dev shell)

# Clean reproducible build:
spark-cli build InternalWebsiteClient

# Deploy:
spark-cli cdk --profile pipeline deploy InternalServiceStack
```

### Change something in BusinessAPI or AppAPI

Same pattern as Internal — just substitute the service chain:

```bash
spark-cli build BusinessModel
spark-cli build BusinessAPILambda
spark-cli cdk --profile pipeline deploy BusinessServiceStack
```

---

## Deploying to Beta

> 🚨 **Never deploy to prod.** Prod is promoted via `main` branch merge → CI/CD pipeline only.

### Standard deploy flow

```bash
# 1. Build first
spark-cli build-all

# 2. List stacks (first time or after infra changes)
spark-cli cdk --profile pipeline list

# 3. Preview changes (always do this before deploying)
spark-cli cdk --profile pipeline diff

# 4. Deploy
spark-cli cdk --profile pipeline deploy InternalServiceStack
```

### Deploy a specific service stack

```bash
spark-cli cdk --profile pipeline deploy InternalServiceStack    # Internal
spark-cli cdk --profile pipeline deploy BusinessServiceStack    # Business
spark-cli cdk --profile pipeline deploy AppServiceStack         # App
spark-cli cdk --profile pipeline deploy CorePipelineStack       # CI/CD pipeline
```

### Deploy everything

```bash
spark-cli cdk --profile pipeline deploy --all
```

### Skip the approval prompt

```bash
spark-cli cdk --profile pipeline deploy InternalServiceStack --require-approval never
```

### AWS SSO session expired?

```bash
aws sso login --profile openclaw-pipeline
spark-cli cdk --profile pipeline deploy ...
```

---

## Creating a PR

```bash
cd ~/SparkRewards/InternalAPILambda
spark-cli pr
```

This:
1. Pushes your branch to `origin`
2. Opens `gh pr create` with the SparkRewards PR template pre-filled
3. Base branch: `flame-develop`

**PR template:**

```markdown
## Description
fix my change     ← inferred from branch name

## Reason
<!-- Why is this change needed? -->

## Testing
- [ ] spark-cli build passes
- [ ] Tested locally
- [ ] Tested on beta (if applicable)
```

**Options:**

```bash
spark-cli pr                              # interactive (prompted for title)
spark-cli pr --title "fix: null check"    # non-interactive
spark-cli pr --draft                      # draft PR (WIP)
```

---

## Checking CI

After creating a PR:

```bash
gh pr checks                # watch CI status
gh pr view --web            # open PR in browser
```

CI runs the same `spark-cli build-all` (via `nix build --impure .#all`) that you ran locally.

**If CI fails:**

1. Check the failing job in GitHub Actions
2. If it's a build failure — run `spark-cli build <package> --verbose` locally to reproduce
3. Fix, push to the same branch, CI re-runs automatically

> 💡 **Tip:** If it builds locally with `spark-cli build` and fails in CI, the most common cause is a missing or expired `GITHUB_TOKEN` in CI, or a repo dependency that needs updating in `flake.lock`.

---

## Merging

PR must meet all of:

- [ ] `spark-cli build-all` passes
- [ ] Tested on beta (`spark-cli cdk --profile pipeline deploy`)
- [ ] All CI checks green (no exceptions)
- [ ] PR description complete (Description + Reason + Testing)
- [ ] At least one reviewer approved

**Never merge with failing CI. No exceptions.**

After merging to `flame-develop`:
- The `flame-develop → main` PR is reviewed separately
- Merging `main` triggers the production pipeline automatically

---

## Updating `flake.lock` (Updating Dependencies)

When you want to pull in the latest commits from GitHub repos:

```bash
cd ~/SparkRewards
nix flake update            # updates all inputs in flake.lock
spark-cli build-all         # verify everything still builds
git add flake.lock
git commit -m "chore: update nix flake.lock"
spark-cli pr
```

> 💡 **Tip:** Update `flake.lock` regularly (weekly or before major releases). Stale lock files can cause merge conflicts and security issues.

---

## Quick Reference

```
Morning:
  spark-cli dev

Make changes:
  git checkout -b feat/my-thing
  # edit files...
  npm run watch              # hot-reload during dev (inside dev shell)

Build:
  spark-cli build InternalAPILambda
  spark-cli build-all

Deploy to beta:
  spark-cli cdk --profile pipeline diff
  spark-cli cdk --profile pipeline deploy InternalServiceStack

Ship code:
  spark-cli pr
  gh pr checks
```
