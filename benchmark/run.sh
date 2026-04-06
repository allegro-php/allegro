#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# Allegro Benchmark Suite
# Compares allegro install vs composer install across projects
#
# Usage:
#   ./benchmark/run.sh [--skip-clone]
#
# Requirements: go, composer (>= 2.0), php, git, python3
# ============================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BENCH_DIR="${BENCH_DIR:-/tmp/allegro-benchmark}"
ALLEGRO_BIN="$BENCH_DIR/allegro"
SKIP_CLONE="${1:-}"

# Projects: name url (small / medium / large / enterprise)
NAMES=(laravel koel matomo spryker)
URLS=(
    "https://github.com/laravel/laravel.git"
    "https://github.com/koel/koel.git"
    "https://github.com/matomo-org/matomo.git"
    "https://github.com/spryker-shop/suite.git"
)
C='\033[0;36m'
B='\033[1m'
N='\033[0m'

log() { echo -e "${C}=== $1 ===${N}"; }
timeit() {
    local s e
    s=$(python3 -c "import time; print(time.time())")
    eval "$@" > /dev/null 2>&1 || true
    e=$(python3 -c "import time; print(time.time())")
    python3 -c "d=$e-$s; print(f'{d:.2f}')"
}

# Setup
log "Setting up"
mkdir -p "$BENCH_DIR"

log "Building Allegro"
cd "$ROOT_DIR"
go build -o "$ALLEGRO_BIN" ./cmd/allegro

if [ "$SKIP_CLONE" != "--skip-clone" ]; then
    for i in "${!NAMES[@]}"; do
        name="${NAMES[$i]}"
        url="${URLS[$i]}"
        dest="$BENCH_DIR/$name"
        if [ -d "$dest" ]; then
            log "Exists: $name"
        else
            log "Cloning $name"
            git clone --depth 1 "$url" "$dest"
        fi
    done
fi

for name in "${NAMES[@]}"; do
    dest="$BENCH_DIR/$name"
    if [ ! -f "$dest/composer.lock" ]; then
        log "Generating lock: $name"
        cd "$dest"
        composer update --no-install --no-scripts --no-interaction 2>&1 | tail -3
    fi
done

log "Projects"
for name in "${NAMES[@]}"; do
    dest="$BENCH_DIR/$name"
    python3 -c "
import json
with open('$dest/composer.lock') as f:
    d = json.load(f)
p = len(d.get('packages', []))
dp = len(d.get('packages-dev', []))
print(f'  $name: {p} + {dp} dev = {p+dp} total')
"
done

echo ""
log "System"
echo "  OS: $(uname -s) $(uname -m)"
echo "  Composer: $(composer --version --no-ansi 2>/dev/null | head -1)"
echo "  PHP: $(php -v 2>/dev/null | head -1 | cut -d' ' -f1-2)"
echo ""

# Benchmark
run_benchmark() {
    local name="$1"
    local dest="$BENCH_DIR/$name"
    local cas="$BENCH_DIR/store-$name"
    cd "$dest"

    log "Benchmarking: $name"

    echo -n "  S1: Cold (no cache)         "
    composer clear-cache > /dev/null 2>&1 || true
    rm -rf vendor
    local s1c; s1c=$(timeit "composer install --no-scripts --no-interaction")
    echo -n "Composer: ${s1c}s  "
    rm -rf vendor "$cas" ~/.allegro
    export ALLEGRO_STORE="$cas"
    local s1a; s1a=$(timeit "$ALLEGRO_BIN install --no-autoload --link-strategy copy")
    echo "Allegro: ${s1a}s"

    echo -n "  S2: Warm cache, no vendor   "
    rm -rf vendor
    local s2c; s2c=$(timeit "composer install --no-scripts --no-interaction")
    echo -n "Composer: ${s2c}s  "
    rm -rf vendor
    local s2a; s2a=$(timeit "$ALLEGRO_BIN install --no-autoload --link-strategy copy")
    echo "Allegro: ${s2a}s"

    echo -n "  S3: Warm + vendor (noop)    "
    local s3c; s3c=$(timeit "composer install --no-scripts --no-interaction")
    echo -n "Composer: ${s3c}s  "
    local s3a; s3a=$(timeit "$ALLEGRO_BIN install --no-autoload --link-strategy copy")
    echo "Allegro: ${s3a}s"

    local vs; vs=$(du -sh vendor 2>/dev/null | cut -f1)
    local ss; ss=$(du -sh "$cas" 2>/dev/null | cut -f1)
    echo "  Disk: vendor=$vs  CAS=$ss"

    echo "$name|$s1c|$s1a|$s2c|$s2a|$s3c|$s3a|$vs|$ss" >> "$BENCH_DIR/results.csv"
    unset ALLEGRO_STORE
    echo ""
}

rm -f "$BENCH_DIR/results.csv"
for name in "${NAMES[@]}"; do
    run_benchmark "$name"
done

# Summary table
echo -e "${B}SUMMARY${N}"
echo ""
printf "%-12s │ %7s │ %7s │ %7s │ %7s │ %7s │ %7s │ %7s │ %7s\n" \
    "Project" "S1-C" "S1-A" "S2-C" "S2-A" "S3-C" "S3-A" "vendor" "CAS"
echo "─────────────┼─────────┼─────────┼─────────┼─────────┼─────────┼─────────┼─────────┼─────────"

while IFS='|' read -r nm s1c s1a s2c s2a s3c s3a vs ss; do
    printf "%-12s │ %6ss │ %6ss │ %6ss │ %6ss │ %6ss │ %6ss │ %7s │ %7s\n" \
        "$nm" "$s1c" "$s1a" "$s2c" "$s2a" "$s3c" "$s3a" "$vs" "$ss"
done < "$BENCH_DIR/results.csv"

echo ""
echo "S1 = Cold (no cache) — network-bound"
echo "S2 = Warm cache, no vendor — key comparison"
echo "S3 = Warm + vendor — reinstall"
echo "C = Composer, A = Allegro"
echo ""
echo "Results: $BENCH_DIR/results.csv"
