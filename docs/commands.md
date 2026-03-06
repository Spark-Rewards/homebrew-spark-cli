# spark-cli Commands Reference

## Setup (New Developer)

```bash
# 1. Install spark-cli
brew tap Spark-Rewards/homebrew-spark-cli
brew install spark-cli

# 2. First-time setup (installs Nix, configures GitHub token, warms cache)
spark-cli setup

# 3. Create workspace
spark-cli workspace create ~/SparkRewards

# 4. Clone repos you need
spark-cli use InternalAPILambda
spark-cli use InternalServiceCDK
spark-cli use InternalWebsiteClient
# ... or clone all at once
spark-cli use --all

# 5. Verify everything works
spark-cli doctor
```

## Daily Workflow

### Building

```bash
# Build current repo
spark-cli run build

# Build a specific repo
spark-cli run build --repo InternalAPILambda

# Build all repos in dependency order
spark-cli build-all
```

### Testing

```bash
# Run tests in current repo
spark-cli run test

# Run tests with options
spark-cli run "npm test -- --watch"

# Type check only
spark-cli run "npx tsc --noEmit"
```

### Deploying (Beta)

```bash
# Deploy a specific CDK stack to beta
spark-cli cdk deploy "Internal-PipelineStack/beta/Internal-Website-Stack"

# Deploy with auto-approve
spark-cli cdk deploy "Internal-PipelineStack/beta/Internal-Website-Stack" --require-approval never

# See what would change before deploying
spark-cli cdk diff "Internal-PipelineStack/beta/Internal-Website-Stack"

# List all stacks
spark-cli cdk list
```

> ⚠️ **Never deploy to prod directly.** Prod deploys happen via PR merge pipeline only.

### Git / Rebasing

```bash
# Always work on flame-develop
git checkout flame-develop

# Rebase on latest main before creating PR
git fetch origin
git rebase origin/main

# If conflicts, resolve then:
git rebase --continue

# Force push after rebase (only on your branch)
git push origin flame-develop --force-with-lease

# Create PR
gh pr create --base main --head flame-develop --title "feat: your feature"
```

## Environment

```bash
# Enter Nix dev shell manually
spark-cli dev

# Run any command inside Nix shell
spark-cli run "node --version"
spark-cli run "java --version"

# Check workspace health
spark-cli doctor

# See which repos are cloned vs available
spark-cli status
```

## Workspace Management

```bash
# Show workspace info
spark-cli workspace info

# Configure AWS profile
spark-cli workspace configure --profile beta

# Add a new repo
spark-cli use BusinessAPILambda

# Remove a repo
spark-cli remove BusinessAPILambda
```

## CDK Commands

```bash
# Synth (generate CloudFormation templates)
spark-cli cdk synth

# Deploy specific stack
spark-cli cdk deploy "StackName"

# Diff (preview changes)
spark-cli cdk diff "StackName"

# Destroy stack (careful!)
spark-cli cdk destroy "StackName"

# List all stacks
spark-cli cdk list
```

## Quick Reference

| Task | Command |
|------|---------|
| Setup workspace | `spark-cli setup` |
| Clone a repo | `spark-cli use <repo>` |
| Build | `spark-cli run build` |
| Test | `spark-cli run test` |
| Deploy to beta | `spark-cli cdk deploy "Stack"` |
| Check health | `spark-cli doctor` |
| Enter dev shell | `spark-cli dev` |
| CDK diff | `spark-cli cdk diff "Stack"` |

## Rules

1. **Never run `cdk`, `npm`, `npx`, or `node` directly** — always use `spark-cli`
2. **Never deploy to prod** — only via merge pipeline
3. **Never commit to main** — PRs from `flame-develop` only
4. **Never merge with failing CI** — green checks required
5. **Always rebase before PR** — `git rebase origin/main`
