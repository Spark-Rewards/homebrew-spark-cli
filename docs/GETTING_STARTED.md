# Getting started with spark-cli (super simple guide)

This doc explains **what spark-cli is** and **how to use it** in plain terms. No prior multi-repo or CLI experience required.

---

## What is spark-cli?

**spark-cli** is a small command-line tool that helps you work with **multiple Git repos in one place** and keeps things like **secrets and build order** under control.

Think of it like this:

- We have lots of separate repos (AppModel, AppAPI, MobileApp, BusinessAPI, etc.).
- You could clone each repo into random folders and run each app’s build by hand—but that gets messy and easy to mess up.
- **spark-cli** gives you one “workspace” folder where all those repos live together, and one set of commands to **clone**, **sync**, **run builds**, and **share environment variables** (like API keys and config).

So: **spark-cli = “workspace manager” for our multi-repo setup.**

---

## Why do we use it?

1. **One place for everything** – All the repos you need sit in one workspace folder (e.g. `~/SparkRewards`).
2. **Shared environment** – One `.env` file at the workspace root; spark-cli can fill it from AWS (SSM) and optionally symlink it into repos so everyone uses the same config.
3. **Smart builds** – When you run `spark-cli run build` in a repo that depends on another (e.g. AppAPI depends on AppModel), spark-cli can build dependencies first and link local packages so you’re not constantly publishing to npm.
4. **Sync and credentials** – `spark-cli sync` updates all repos (git fetch + rebase) and refreshes the workspace `.env` from AWS, and can trigger AWS SSO login if needed.

So: **spark-cli keeps multi-repo development consistent and less error-prone.**

---

## Install

You need **Homebrew** (macOS or Linux).

```bash
brew tap Spark-Rewards/spark-cli
brew install spark-cli
```

Check it’s there:

```bash
spark-cli --help
```

---

## How to use it (step by step)

### 1. Create a workspace

A **workspace** is just a folder that spark-cli “owns”: it will put a `.spark-cli` directory inside it (with `workspace.json`) and later a `.env` file there.

```bash
spark-cli create workspace ~/SparkRewards
```

- Use any path you want (e.g. `./my-dev`, `~/Projects/SparkRewards`).
- If the folder doesn’t exist, spark-cli creates it.
- Don’t run this inside an existing repo; create the workspace first, then add repos into it.

**What happened:** You now have a folder (e.g. `~/SparkRewards`) with a `.spark-cli/workspace.json` file. That’s your workspace root.

---

### 2. Go into the workspace

```bash
cd ~/SparkRewards
```

All spark-cli commands that “need a workspace” look for `.spark-cli/workspace.json` in the current directory or any parent. So from now on, run spark-cli from inside this folder (or inside any repo under it).

---

### 3. Add repos with `spark-cli use`

“Use” here means “add this repo to my workspace” (clone it if needed and register it in `workspace.json`).

```bash
spark-cli use AppModel
spark-cli use AppAPI
spark-cli use MobileApp
```

- Repo name only (e.g. `AppModel`) → spark-cli assumes **Spark-Rewards** org and clones `Spark-Rewards/AppModel`.
- You can use `org/repo` or a full Git URL if needed.

Each repo ends up as a subfolder: e.g. `~/SparkRewards/AppModel`, `~/SparkRewards/AppAPI`.

---

### 4. Sync repos and environment: `spark-cli sync`

This does two things:

1. **Git:** For each repo in the workspace, it does fetch + rebase (so you’re up to date with the remote).
2. **Environment:** It pulls values from AWS (SSM) and writes them into the workspace `.env` file. If your AWS session is expired, it can prompt you to log in (SSO).

```bash
spark-cli sync
```

- **First time or after long idle:** You may need to log in to AWS. Run `spark-cli login` if you want to do that separately, or let `spark-cli sync` prompt you.
- **Only sync one repo:** `spark-cli sync AppAPI`
- **Skip refreshing .env:** `spark-cli sync --no-env`
- **Use a different env (e.g. prod):** `spark-cli sync --env prod`

So: **sync = “update all my repos and refresh my shared .env from AWS.”**

---

### 5. Run scripts with `spark-cli run`

When you’re **inside a repo directory** (e.g. `~/SparkRewards/AppAPI`), you can run scripts through spark-cli so it can inject the workspace `.env` and handle dependencies.

```bash
cd ~/SparkRewards/AppAPI
spark-cli run build
```

- spark-cli figures out the project type (Node/npm, Gradle, Go, Make) and runs the right command (e.g. `npm run build`, `./gradlew build`).
- **List scripts:** Run `spark-cli run` with no arguments to see what’s available in the current repo.
- **Build with dependencies:** If this repo depends on another (e.g. AppModel), use:
  ```bash
  spark-cli run build -r
  ```
  The `-r` (recursive) flag builds dependency repos first, then the current one, and can link local packages so you don’t need to publish to npm.

So: **spark-cli run = “run a script in this repo, with workspace env and optional dependency building.”**

---

## Command cheat sheet (plain English)

| Command | What it does |
|--------|----------------|
| `spark-cli create workspace <path>` | Create a new workspace folder; spark-cli will put `.spark-cli/workspace.json` there. |
| `spark-cli use <repo>` | Clone the repo into the workspace (if needed) and add it to the manifest. e.g. `spark-cli use AppAPI`. |
| `spark-cli sync` | Update all workspace repos (fetch + rebase) and refresh the shared `.env` from AWS. |
| `spark-cli sync <repo>` | Same as above but only for that repo. |
| `spark-cli run` | (Inside a repo.) List available scripts (e.g. build, test, start). |
| `spark-cli run <script>` | (Inside a repo.) Run that script with workspace env; e.g. `spark-cli run build`, `spark-cli run test`. |
| `spark-cli run build -r` | Build this repo after building its dependencies; uses local linked packages when possible. |
| `spark-cli info` | Show workspace name, path, repos, their branches and status (clean/dirty). Aliases: `spark-cli status`, `spark-cli ws`. |
| `spark-cli env` | Show current workspace environment variables (from the root `.env`). |
| `spark-cli env set KEY=value` | Set (or overwrite) a variable in the workspace `.env`. |
| `spark-cli env link` | Symlink each repo’s `.env` to the workspace `.env` so all repos share the same env. |
| `spark-cli login` | Log in to AWS SSO (e.g. when `spark-cli sync` says your session expired). |
| `spark-cli remove <repo>` | Remove the repo from the workspace manifest only; does **not** delete the repo folder. |

---

## Where things live

- **Workspace root:** The folder you created with `spark-cli create workspace <path>` (e.g. `~/SparkRewards`).
- **`.spark-cli/workspace.json`** – List of repos and workspace settings (AWS profile, region, etc.). Don’t edit by hand unless you know what you’re doing.
- **`.env`** – At the workspace root. Filled by `spark-cli sync` from AWS (and optionally by `spark-cli env set`). Used when you run `spark-cli run` so scripts see the same config.
- **Repos** – Each one is a normal Git repo in a subfolder (e.g. `AppAPI`, `AppModel`).

---

## Tips for newbies

1. **Always start from the workspace root** when you run `spark-cli use` or `spark-cli sync`. For `spark-cli run`, you must be inside a repo directory.
2. **“Not inside a spark-cli workspace”** – You’re not in the workspace folder (or a subfolder of it). `cd` to your workspace root and try again.
3. **First time AWS** – If `spark-cli sync` or `spark-cli login` fails, you may need to run `aws configure sso` once; spark-cli will show instructions if SSO isn’t set up.
4. **Build failures** – If a build needs another repo (e.g. AppAPI needs AppModel), try `spark-cli run build -r` from the repo that’s failing.
5. **VS Code** – spark-cli can generate a multi-root workspace file (e.g. `SparkRewards.code-workspace`) so you can open all repos in one VS Code window. It’s created/updated when you create the workspace and when you add repos or run `spark-cli sync`.

---

## Quick start (copy-paste)

```bash
# Install
brew tap Spark-Rewards/spark-cli
brew install spark-cli

# Create workspace and add repos
spark-cli create workspace ~/SparkRewards
cd ~/SparkRewards
spark-cli use AppModel
spark-cli use AppAPI
spark-cli use MobileApp

# Sync everything and refresh .env (will prompt for AWS if needed)
spark-cli sync

# Build a repo (from inside that repo)
cd AppAPI
spark-cli run build
# Or build with dependencies first:
spark-cli run build -r
```

That’s it. For more, run `spark-cli --help` and `spark-cli <command> --help`.
