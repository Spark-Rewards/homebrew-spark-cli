#!/usr/bin/env bash
# ══════════════════════════════════════════════════════════════════════════════
# SparkRewards — One-Shot Developer Setup (bash fallback)
#
# PRIMARY PATH: Use 'spark-cli setup' instead of this script.
# This script exists as a bootstrap fallback for before spark-cli is installed.
#
# Idempotent: safe to run multiple times. Skips steps already done.
#
# USAGE:
#   spark-cli setup             # preferred (uses spark-cli)
#   ./setup.sh                  # fallback (bash, no spark-cli required)
#   ./setup.sh --skip-cache     # Skip devShell pre-cache (faster, less useful)
# ══════════════════════════════════════════════════════════════════════════════

set -euo pipefail

# ── Parse flags ────────────────────────────────────────────────────────────────
SKIP_CACHE=0
for arg in "$@"; do
  case "$arg" in
    --skip-cache) SKIP_CACHE=1 ;;
    --help|-h)
      echo "Usage: ./setup.sh [--skip-cache]"
      echo ""
      echo "  --skip-cache   Skip pre-warming the Nix devShell (faster, but first 'spark-nix dev' will be slower)"
      exit 0
      ;;
  esac
done

# ── Colors ─────────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m' # No Color

# ── Helpers ────────────────────────────────────────────────────────────────────
step()    { echo ""; echo -e "${BOLD}${BLUE}▶ $*${NC}"; }
ok()      { echo -e "  ${GREEN}✓${NC} $*"; }
skip()    { echo -e "  ${DIM}⊘ $* (already done)${NC}"; }
warn()    { echo -e "  ${YELLOW}⚠${NC}  $*"; }
err()     { echo -e "  ${RED}✗${NC} $*" >&2; }
info()    { echo -e "  ${DIM}→${NC} $*"; }
fatal()   { err "$*"; echo ""; exit 1; }

# ── Banner ─────────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║   🔥 SparkRewards Developer Setup                ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
echo ""
echo "This script will:"
echo "  1. Install Nix (Determinate Systems)"
echo "  2. Enable Nix flakes"
echo "  3. Configure GitHub token for private repos"
echo "  4. Install spark-nix command"
echo "  5. Pre-warm the dev environment"
echo ""

# ── Detect workspace ───────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ ! -f "$SCRIPT_DIR/flake.nix" ]; then
  fatal "Must be run from the SparkRewards workspace (no flake.nix found in $SCRIPT_DIR)"
fi
WORKSPACE="$SCRIPT_DIR"
info "Workspace: $WORKSPACE"

# ── OS check ───────────────────────────────────────────────────────────────────
OS="$(uname -s)"
ARCH="$(uname -m)"
if [ "$OS" != "Darwin" ] && [ "$OS" != "Linux" ]; then
  fatal "Unsupported OS: $OS. SparkRewards supports macOS and Linux."
fi
info "Platform: $OS/$ARCH"
echo ""

# ══════════════════════════════════════════════════════════════════════════════
# Step 1: Install Nix
# ══════════════════════════════════════════════════════════════════════════════
step "Step 1/5: Checking Nix installation"

NIX_BIN="/nix/var/nix/profiles/default/bin"
NIX_CMD="$NIX_BIN/nix"

if [ -x "$NIX_CMD" ]; then
  NIX_VERSION=$("$NIX_CMD" --version 2>/dev/null || echo "unknown")
  skip "Nix already installed ($NIX_VERSION)"
else
  echo ""
  warn "Nix is not installed. Installing via Determinate Systems..."
  warn "This will require sudo access and takes ~2 minutes."
  echo ""
  echo -n "  Proceed? [Y/n] "
  read -r answer
  answer="${answer:-Y}"
  if [ "$answer" != "Y" ] && [ "$answer" != "y" ]; then
    fatal "Aborted. Install Nix manually: https://install.determinate.systems/"
  fi

  echo ""
  info "Downloading and running Determinate Systems installer..."

  # Install Nix
  # --no-confirm: non-interactive
  # sandbox = false: required on macOS and CodeBuild (no Linux namespaces)
  curl --proto '=https' --tlsv1.2 -sSf \
    https://install.determinate.systems/nix \
    | sh -s -- install \
      --no-confirm \
      --extra-conf "sandbox = false" \
      --extra-conf "experimental-features = nix-command flakes" \
    2>&1 | grep -v "^$" | head -30 || true

  # Source the profile
  if [ -f "/nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh" ]; then
    # shellcheck source=/dev/null
    source "/nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh" || true
  fi

  if [ -x "$NIX_CMD" ]; then
    ok "Nix installed: $($NIX_CMD --version)"
  else
    fatal "Nix installation failed. Try manually: https://install.determinate.systems/"
  fi
fi

# Ensure Nix is in PATH for rest of script
export PATH="$NIX_BIN:$PATH"

# ══════════════════════════════════════════════════════════════════════════════
# Step 2: Enable Nix Flakes
# ══════════════════════════════════════════════════════════════════════════════
step "Step 2/5: Enabling Nix flakes"

NIX_CONF_DIR="$HOME/.config/nix"
NIX_CONF="$NIX_CONF_DIR/nix.conf"

mkdir -p "$NIX_CONF_DIR"

if [ -f "$NIX_CONF" ] && grep -q "experimental-features" "$NIX_CONF" 2>/dev/null; then
  skip "Flakes already enabled in $NIX_CONF"
else
  echo "experimental-features = nix-command flakes" >> "$NIX_CONF"
  ok "Flakes enabled in $NIX_CONF"
fi

# ══════════════════════════════════════════════════════════════════════════════
# Step 3: Configure GitHub Token
# ══════════════════════════════════════════════════════════════════════════════
step "Step 3/5: Configuring GitHub token for private repos"

# Check if already configured
if grep -q "access-tokens = github.com=" "$NIX_CONF" 2>/dev/null; then
  skip "GitHub token already in $NIX_CONF"
else
  # Try to get token automatically
  GITHUB_TOKEN=""

  # Method 1: gh CLI (preferred — already authenticated)
  if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    GITHUB_TOKEN=$(gh auth token 2>/dev/null || echo "")
    if [ -n "$GITHUB_TOKEN" ]; then
      info "Using GitHub token from 'gh auth'"
    fi
  fi

  # Method 2: Environment variable
  if [ -z "$GITHUB_TOKEN" ] && [ -n "${GITHUB_TOKEN_ENV:-}" ]; then
    GITHUB_TOKEN="$GITHUB_TOKEN_ENV"
    info "Using GitHub token from GITHUB_TOKEN_ENV environment variable"
  fi

  # Method 3: Prompt user
  if [ -z "$GITHUB_TOKEN" ]; then
    echo ""
    echo "  SparkRewards repos are private. A GitHub Personal Access Token is required."
    echo "  Token needs scopes: read:packages, repo (read)"
    echo ""
    echo "  Get one at: https://github.com/settings/tokens/new"
    echo "  Or run: gh auth login (if you have GitHub CLI installed)"
    echo ""
    echo -n "  GitHub token (or press Enter to skip): "
    read -r -s GITHUB_TOKEN
    echo ""
  fi

  if [ -n "$GITHUB_TOKEN" ]; then
    # Remove any existing access-tokens line before adding
    if [ -f "$NIX_CONF" ]; then
      grep -v "^access-tokens" "$NIX_CONF" > "$NIX_CONF.tmp" 2>/dev/null && mv "$NIX_CONF.tmp" "$NIX_CONF" || true
    fi
    echo "access-tokens = github.com=$GITHUB_TOKEN" >> "$NIX_CONF"
    ok "GitHub token configured in $NIX_CONF"

    # Also configure npm for GitHub Packages (needed for @spark-rewards/* npm packages)
    # This sets it globally so npm install works for private packages
    npm_conf_dir="$HOME/.npmrc"
    if ! grep -q "npm.pkg.github.com" "$npm_conf_dir" 2>/dev/null; then
      {
        echo "@spark-rewards:registry=https://npm.pkg.github.com"
        echo "//npm.pkg.github.com/:_authToken=$GITHUB_TOKEN"
      } >> "$npm_conf_dir"
      ok "npm configured for GitHub Packages registry (~/.npmrc)"
    else
      skip "npm already configured for GitHub Packages"
    fi
  else
    warn "Skipped GitHub token — remote repo fetching will not work until configured"
    warn "Add manually to $NIX_CONF:"
    warn "  access-tokens = github.com=<YOUR_TOKEN>"
  fi
fi

# Also set GITHUB_TOKEN env var for this session (npm builds need it)
if [ -z "${GITHUB_TOKEN:-}" ]; then
  if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    export GITHUB_TOKEN=$(gh auth token 2>/dev/null || echo "")
  fi
fi

# ══════════════════════════════════════════════════════════════════════════════
# Step 4: Install spark-nix command
# ══════════════════════════════════════════════════════════════════════════════
step "Step 4/5: Installing spark-nix command"

SPARK_NIX_SH="$WORKSPACE/spark-nix.sh"
if [ ! -f "$SPARK_NIX_SH" ]; then
  fatal "spark-nix.sh not found at $SPARK_NIX_SH"
fi

# Make executable
chmod +x "$SPARK_NIX_SH"

# Try to install to /usr/local/bin (system-wide)
INSTALL_TARGET="/usr/local/bin/spark-nix"
FALLBACK_DIR="$HOME/.local/bin"
FALLBACK_TARGET="$FALLBACK_DIR/spark-nix"

if [ -L "$INSTALL_TARGET" ] && [ "$(readlink "$INSTALL_TARGET")" = "$SPARK_NIX_SH" ]; then
  skip "spark-nix already installed at $INSTALL_TARGET"
elif [ -L "$FALLBACK_TARGET" ] && [ "$(readlink "$FALLBACK_TARGET")" = "$SPARK_NIX_SH" ]; then
  skip "spark-nix already installed at $FALLBACK_TARGET"
else
  # Try /usr/local/bin first (may need sudo)
  if ln -sf "$SPARK_NIX_SH" "$INSTALL_TARGET" 2>/dev/null; then
    ok "spark-nix installed → $INSTALL_TARGET"
  elif sudo ln -sf "$SPARK_NIX_SH" "$INSTALL_TARGET" 2>/dev/null; then
    ok "spark-nix installed → $INSTALL_TARGET (via sudo)"
  else
    # Fall back to ~/.local/bin
    mkdir -p "$FALLBACK_DIR"
    ln -sf "$SPARK_NIX_SH" "$FALLBACK_TARGET"
    ok "spark-nix installed → $FALLBACK_TARGET"

    # Check if ~/.local/bin is in PATH
    if ! echo "$PATH" | grep -q "$FALLBACK_DIR"; then
      warn "Add ~/.local/bin to your PATH to use spark-nix from anywhere:"
      warn "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc"
      warn "  source ~/.zshrc"
    fi
  fi
fi

# ══════════════════════════════════════════════════════════════════════════════
# Step 5: Pre-warm devShell
# ══════════════════════════════════════════════════════════════════════════════
step "Step 5/5: Pre-warming dev environment"

if [ "$SKIP_CACHE" = "1" ]; then
  skip "Skipped (--skip-cache)"
else
  echo "  Downloading toolchain packages (Node 22, Java 17, AWS CLI...)"
  echo "  This takes 3-10 minutes on first run, then is instant. ☕"
  echo ""

  cd "$WORKSPACE"
  "$NIX_CMD" develop --impure --command echo "  → Dev shell verified ✓" 2>&1

  ok "Dev environment pre-warmed and cached"
fi

# ══════════════════════════════════════════════════════════════════════════════
# Done!
# ══════════════════════════════════════════════════════════════════════════════
echo ""
echo -e "${BOLD}${GREEN}╔══════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${GREEN}║   ✅ Setup Complete!                              ║${NC}"
echo -e "${BOLD}${GREEN}╚══════════════════════════════════════════════════╝${NC}"
echo ""
echo "  Next steps:"
echo ""
echo -e "    ${BOLD}spark-nix dev${NC}                          # Enter dev shell"
echo -e "    ${BOLD}spark-nix status${NC}                       # See local vs remote repos"
echo -e "    ${BOLD}spark-nix checkout InternalAPILambda${NC}   # Clone a repo to work on"
echo -e "    ${BOLD}spark-nix build-all${NC}                    # Build everything"
echo ""
echo -e "  ${DIM}Run 'spark-nix help' for all commands.${NC}"
echo ""
