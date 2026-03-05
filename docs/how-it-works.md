# How It Works — Nix Build System

SparkRewards uses [Nix](https://nixos.org/) for reproducible builds. This document explains the design, what the flake does, and how `spark-cli` fits in.

---

## Why Nix?

The classic problem: *"it works on my machine."*

- Developer A has Node 18, Developer B has Node 20, CI has Node 22. Code breaks in CI.
- "Just `brew install` these 6 tools" → different versions, different behavior.
- Docker is heavy and slow for local dev loops.

Nix solves this by making the build environment itself a reproducible artifact:

- **Same inputs → same outputs.** Every machine, every time.
- **No global installs.** Node, Java, AWS CLI are provided by Nix, not by `brew`.
- **Instant switching.** Different projects can use different toolchain versions without conflict.
- **CI = local.** The dev shell is identical to what CodeBuild uses.

---

## The Brazil Analogy

The design is inspired by Amazon's [Brazil build system](https://blog.acolyer.org/2020/03/13/build-and-dev-tools-at-amazon/).

**Core concept:** Clone only the repos you're actively editing. Everything else is fetched automatically.

```
You have locally:            InternalAPILambda  (editing)
Nix fetches from GitHub:     InternalModel, InternalWebsiteClient, InternalServiceCDK
```

You never have to clone all 14 repos to build anything. The Nix flake resolves dependencies transparently.

---

## The Workspace Flake

`~/SparkRewards/flake.nix` is the heart of the build system.

### Inputs — pinned repo versions

Every SparkRewards repo is a flake input, pointing to GitHub:

```nix
inputs = {
  nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  internal-model-src = {
    url = "github:Spark-Rewards/InternalModel";
    flake = false;   # just the source tree, not a full flake
  };
  internal-api-lambda-src = {
    url = "github:Spark-Rewards/InternalAPILambda";
    flake = false;
  };
  # ... 12 more repos ...
};
```

`flake.lock` pins every input to a specific git commit hash. This is what makes builds reproducible — the exact same source code is used every time until you explicitly update the lock.

### The `localOrGitHub` function — sparse checkout

This is the key to the Brazil-like behavior:

```nix
localOrGitHub = { name, githubSrc, ... }:
  let
    localPath = workspaceRoot + "/${name}";
    localExists = builtins.pathExists localPath;
  in
  if localExists
  then builtins.path { path = localPath; ... }  # use your local clone
  else githubSrc;                                # use the pinned GitHub version
```

At evaluation time, Nix checks if `~/SparkRewards/<RepoName>` exists. If it does, your local files are used. If not, the pinned GitHub source from `flake.lock` is used.

This is why `--impure` is required — Nix normally evaluates in a pure, sandboxed environment and can't read your filesystem. The `--impure` flag allows `builtins.pathExists` and `builtins.getEnv` to work.

### Build derivations

The flake defines four builder templates:

| Template | Used for | Build tool |
|----------|----------|-----------|
| `mkModelDerivation` | `*Model` repos | Gradle + Smithy |
| `mkLambdaDerivation` | `*APILambda` repos | TypeScript + tsc |
| `mkWebsiteDerivation` | `*Website` repos | TypeScript + Vite |
| `mkCDKDerivation` | `*ServiceCDK` repos | TypeScript + tsc |

Each derivation is a Nix function that declares:
- **inputs** (`nativeBuildInputs`): which tools are needed (Node 22, Java 17, etc.)
- **buildPhase**: shell script that runs the build
- **installPhase**: shell script that copies outputs to the Nix store

Example (simplified):

```nix
mkLambdaDerivation = { name, src, displayName }:
  pkgs.stdenv.mkDerivation {
    inherit name src;
    nativeBuildInputs = with pkgs; [ nodejs_22 cacert ];
    __noChroot = true;                 # allows npm install to hit the network
    GITHUB_TOKEN = builtins.getEnv "GITHUB_TOKEN";

    buildPhase = ''
      if [ -d "node_modules" ]; then
        echo "Using pre-installed node_modules from local clone"
      else
        npm install   # downloads @spark-rewards/* from GitHub Packages
      fi
      tsc -p tsconfig.json
    '';

    installPhase = ''
      mkdir -p $out/lambda
      cp -r dist/. $out/lambda/
    '';
  };
```

### Output packages

The flake exposes named packages:

```nix
packages.aarch64-darwin = {
  internal-model          = mkModelDerivation   { ... };
  internal-api-lambda     = mkLambdaDerivation  { ... };
  internal-website-client = mkWebsiteDerivation { ... };
  internal-service-cdk    = mkCDKDerivation     { ... };

  # Convenience bundles:
  internal-all  = pkgs.symlinkJoin { paths = [ internal-model internal-api-lambda ... ]; };
  business-all  = pkgs.symlinkJoin { paths = [ ... ]; };
  all           = pkgs.symlinkJoin { paths = [ internal-all business-all app-all ... ]; };
};
```

### Dev shell

```nix
devShells.aarch64-darwin.default = pkgs.mkShell {
  packages = with pkgs; [
    nodejs_22           # exact version, always
    jdk17               # exact version, always
    awscli2
    gradle
    nodePackages.typescript
    jq curl git
  ];

  shellHook = ''
    echo "🔥 SparkRewards — Dev Shell"
    node --version; java -version; aws --version; gradle --version
  '';
};
```

This shell is activated by `nix develop --impure`.

---

## How `spark-cli` Wraps Nix

`spark-cli` is a Go binary that wraps `nix` commands and adds:
1. Workspace discovery (`FindWorkspace` walks up to find `flake.nix`)
2. PATH injection (Nix binary is at `/nix/var/nix/profiles/default/bin/nix`)
3. `GITHUB_TOKEN` injection from `~/.config/nix/nix.conf` or `gh auth token`
4. Output filtering (Nix is verbose — spark-cli shows only meaningful lines)
5. Friendly error messages and recovery hints

### `spark-cli build InternalAPILambda`

Under the hood:

```bash
# 1. Find workspace (flake.nix)
WORKSPACE=~/SparkRewards

# 2. Map repo name to Nix attribute
InternalAPILambda → internal-api-lambda

# 3. Run nix build with injected env
cd $WORKSPACE
PATH=/nix/var/nix/profiles/default/bin:$PATH
GITHUB_TOKEN=<from nix.conf>
nix build --impure .#internal-api-lambda --print-build-logs
```

Nix evaluates `flake.nix`, checks whether `~/SparkRewards/InternalAPILambda` exists, builds the derivation, and writes output to `/nix/store/<hash>-internal-api-lambda`. The `./result` symlink points there.

### `spark-cli dev`

```bash
cd ~/SparkRewards
nix develop --impure
```

Activates the `devShells.aarch64-darwin.default` shell from the flake.

### Output filtering

Nix is very verbose by default (downloading store paths, evaluating derivations, etc.). `spark-cli` filters the output to show only:

- `[spark]` trace messages (custom messages from the build scripts)
- `error:` and `warning:` lines
- `building <package>` lines (reformatted as `⚙ Building <name>...`)

Everything else (store paths, download progress, evaluation traces) is dropped.

---

## Build Flow — End to End

```
spark-cli build InternalAPILambda
         │
         ▼
  FindWorkspace()  →  ~/SparkRewards/flake.nix found
         │
         ▼
  RepoToPackage()  →  "InternalAPILambda" → "internal-api-lambda"
         │
         ▼
  nix build --impure .#internal-api-lambda
         │
         ▼
  Nix evaluates flake.nix
         │
         ├── localOrGitHub("InternalAPILambda")
         │     ├── ~/SparkRewards/InternalAPILambda exists?
         │     │     YES → use local source tree
         │     │     NO  → use github:Spark-Rewards/InternalAPILambda@<flake.lock hash>
         │     └── returns: source tree
         │
         ├── mkLambdaDerivation { src = <source tree> }
         │     ├── nativeBuildInputs = [nodejs_22, cacert]
         │     ├── buildPhase:
         │     │     - check for node_modules (use if present, else npm install)
         │     │     - tsc -p tsconfig.json
         │     └── installPhase:
         │           - cp dist/ → $out/lambda/
         │
         ▼
  /nix/store/<hash>-internal-api-lambda/
         │
         ▼
  ./result  →  symlink to store path
         │
         ▼
  spark-cli: "✓ Build complete (18s) → ./result"
```

---

## Dependency Graph

```
*Model (Gradle + Smithy codegen)
  → generates TypeScript SDK package
       │
       ├── *APILambda (TypeScript)
       │     → builds Lambda handler bundle
       │
       ├── *Website (TypeScript + Vite)    [Internal, Business only]
       │     → builds frontend static bundle
       │
       └── *ServiceCDK (TypeScript CDK)
             → builds CDK app (uses Lambda bundle)
                   │
                   └── CorePipeline (CI/CD)
```

Three independent service chains (Internal, Business, App) follow the same structure. Build one chain or all at once.

---

## The Nix Store

Everything Nix builds goes into `/nix/store/`. Store paths are content-addressed — the hash is derived from the build inputs. This means:

- If nothing changed, Nix reuses the cached result (instant).
- If a dependency changed, Nix rebuilds only what's affected.
- Different projects can use different versions of the same tool without conflict.
- You can roll back by pointing at an older store path.

```bash
ls -la ~/SparkRewards/result
# result -> /nix/store/rpr4xjs…-internal-api-lambda

ls ~/SparkRewards/result/lambda/
# handler.js  operations/  ...
```

---

## CI/CD Integration

`buildspec-nix.yml` uses the same `flake.nix` as local development:

```yaml
phases:
  build:
    commands:
      - nix build --impure .#all --print-build-logs
```

The `GITHUB_TOKEN` environment variable is injected from AWS Secrets Manager. Everything else is identical to local builds.

---

## Updating Dependencies

To update all repos to their latest `main` commits:

```bash
cd ~/SparkRewards
nix flake update    # updates flake.lock
git add flake.lock
git commit -m "chore: update nix flake.lock"
```

> 💡 **Tip:** `flake.lock` should always be committed. It's what makes builds reproducible across machines and over time.
