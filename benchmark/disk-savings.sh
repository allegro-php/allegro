#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# Allegro Disk Savings Benchmark
# Shows CAS deduplication across N projects with shared deps
#
# Usage:
#   ./benchmark/disk-savings.sh [--skip-clone] [N]
#
# Default: 10 projects. Requirements: go, composer, php, git
# ============================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BENCH_DIR="${BENCH_DIR:-/tmp/allegro-disk-benchmark}"
ALLEGRO_BIN="$BENCH_DIR/allegro"
SKIP_CLONE="${1:-}"
N="${2:-10}"

# If first arg is a number, treat it as N
if [[ "$SKIP_CLONE" =~ ^[0-9]+$ ]]; then
    N="$SKIP_CLONE"
    SKIP_CLONE=""
fi

C='\033[0;36m'
B='\033[1m'
G='\033[0;32m'
N_COLOR='\033[0m'

log() { echo -e "${C}=== $1 ===${N_COLOR}"; }

log "Disk Savings Benchmark: $N Laravel projects"
mkdir -p "$BENCH_DIR"

# Build Allegro
log "Building Allegro"
cd "$ROOT_DIR"
go build -o "$ALLEGRO_BIN" ./cmd/allegro

# Clone template project once
TEMPLATE="$BENCH_DIR/laravel-template"
if [ "$SKIP_CLONE" != "--skip-clone" ] || [ ! -d "$TEMPLATE" ]; then
    if [ ! -d "$TEMPLATE" ]; then
        log "Cloning Laravel template"
        git clone --depth 1 https://github.com/laravel/laravel.git "$TEMPLATE"
    fi
fi

# Generate lock if missing
if [ ! -f "$TEMPLATE/composer.lock" ]; then
    log "Generating composer.lock"
    cd "$TEMPLATE"
    composer update --no-install --no-scripts --no-interaction 2>&1 | tail -3
fi

# Count packages
PKG_COUNT=$(python3 -c "
import json
with open('$TEMPLATE/composer.lock') as f:
    d = json.load(f)
p = len(d.get('packages', []))
dp = len(d.get('packages-dev', []))
print(f'{p} + {dp} dev = {p+dp} total')
")
echo "  Packages: $PKG_COUNT"
echo ""

# ── Scenario 1: Composer (N separate vendor dirs) ──
log "Scenario 1: Composer — $N projects with separate vendor/"

COMPOSER_DIR="$BENCH_DIR/composer-projects"
rm -rf "$COMPOSER_DIR"
mkdir -p "$COMPOSER_DIR"

for i in $(seq 1 "$N"); do
    dest="$COMPOSER_DIR/project-$i"
    cp -R "$TEMPLATE" "$dest"
    rm -rf "$dest/.git" "$dest/vendor"
    cd "$dest"
    composer install --no-scripts --no-interaction --quiet 2>/dev/null
    printf "  Project %2d: installed\n" "$i"
done

COMPOSER_TOTAL=$(du -sm "$COMPOSER_DIR" | cut -f1)
echo ""
echo -e "  ${B}Composer total: ${COMPOSER_TOTAL} MB${N_COLOR} ($N projects)"

# ── Scenario 2: Allegro with shared CAS ──
log "Scenario 2: Allegro — $N projects with shared CAS"

ALLEGRO_DIR="$BENCH_DIR/allegro-projects"
SHARED_CAS="$BENCH_DIR/shared-store"
rm -rf "$ALLEGRO_DIR" "$SHARED_CAS"
mkdir -p "$ALLEGRO_DIR"
export ALLEGRO_STORE="$SHARED_CAS"

for i in $(seq 1 "$N"); do
    dest="$ALLEGRO_DIR/project-$i"
    cp -R "$TEMPLATE" "$dest"
    rm -rf "$dest/.git" "$dest/vendor"
    cd "$dest"
    "$ALLEGRO_BIN" install --no-autoload --no-scripts --link-strategy hardlink --quiet 2>/dev/null
    printf "  Project %2d: installed\n" "$i"
done

ALLEGRO_VENDORS=$(du -sm "$ALLEGRO_DIR" | cut -f1)
ALLEGRO_CAS=$(du -sm "$SHARED_CAS" | cut -f1)
# Actual disk usage (hardlinks share inodes)
ALLEGRO_ACTUAL=$(du -sm "$ALLEGRO_DIR" "$SHARED_CAS" | tail -1 | cut -f1)
# Use a combined du to count shared inodes once
ALLEGRO_REAL=$(du -sm "$ALLEGRO_DIR" "$SHARED_CAS" 2>/dev/null | awk '{s+=$1} END{print s}')
# But for hardlinks, the real disk usage is just the CAS + non-file overhead
# Let's measure with --apparent-size vs actual
ALLEGRO_APPARENT=$(du -sm --apparent-size "$ALLEGRO_DIR" 2>/dev/null | cut -f1 || du -sm "$ALLEGRO_DIR" | cut -f1)

unset ALLEGRO_STORE

echo ""
echo -e "  ${B}Allegro vendors: ${ALLEGRO_VENDORS} MB${N_COLOR} (apparent, $N projects)"
echo -e "  ${B}Allegro CAS:     ${ALLEGRO_CAS} MB${N_COLOR} (shared store)"

# ── Summary ──
echo ""
echo -e "${B}SUMMARY${N_COLOR}"
echo "─────────────────────────────────────────────────"
printf "  %-30s %6s MB\n" "Composer ($N × vendor/):" "$COMPOSER_TOTAL"
printf "  %-30s %6s MB\n" "Allegro ($N × vendor/ + CAS):" "$ALLEGRO_REAL"
echo "─────────────────────────────────────────────────"

if [ "$COMPOSER_TOTAL" -gt 0 ]; then
    SAVED=$((COMPOSER_TOTAL - ALLEGRO_REAL))
    PCT=$(python3 -c "print(f'{($SAVED/$COMPOSER_TOTAL)*100:.0f}')")
    echo -e "  ${G}Disk saved: ${SAVED} MB (${PCT}%)${N_COLOR}"
    echo ""
    echo "  With hardlinks, all $N vendor/ dirs share the same"
    echo "  file inodes from the CAS. Only one copy on disk."
fi

echo ""
echo "  Paths:"
echo "    Composer: $COMPOSER_DIR"
echo "    Allegro:  $ALLEGRO_DIR"
echo "    CAS:      $SHARED_CAS"
