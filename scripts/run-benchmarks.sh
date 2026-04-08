#!/bin/bash
# APICerebrus Benchmark Runner Script
# This script runs comprehensive benchmarks with profiling support

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
BENCH_TIME="1s"
BENCH_COUNT=1
OUTPUT_DIR="benchmark-results"
CPU_PROFILE=false
MEM_PROFILE=false
BLOCK_PROFILE=false
MUTEX_PROFILE=false
RUN_GATEWAY=true
RUN_PLUGIN=true
RUN_STORE=true
VERBOSE=false

# Print usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Run APICerebrus benchmarks with optional profiling.

OPTIONS:
    -h, --help              Show this help message
    -t, --time DURATION     Benchmark duration (default: 1s)
    -c, --count N           Number of benchmark runs (default: 1)
    -o, --output DIR        Output directory for results (default: benchmark-results)
    --cpu                   Enable CPU profiling
    --mem                   Enable memory profiling
    --block                 Enable block profiling
    --mutex                 Enable mutex profiling
    --all-profiles          Enable all profile types
    --gateway-only          Run only gateway benchmarks
    --plugin-only           Run only plugin benchmarks
    --store-only            Run only store benchmarks
    -v, --verbose           Enable verbose output

EXAMPLES:
    $0                              # Run all benchmarks with defaults
    $0 -t 5s -c 3                   # Run for 5 seconds, 3 times
    $0 --cpu --mem -t 10s           # Run with CPU and memory profiling
    $0 --gateway-only --cpu         # Run only gateway benchmarks with CPU profiling
    $0 --all-profiles -t 30s        # Full profiling run for 30 seconds

EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            usage
            exit 0
            ;;
        -t|--time)
            BENCH_TIME="$2"
            shift 2
            ;;
        -c|--count)
            BENCH_COUNT="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --cpu)
            CPU_PROFILE=true
            shift
            ;;
        --mem)
            MEM_PROFILE=true
            shift
            ;;
        --block)
            BLOCK_PROFILE=true
            shift
            ;;
        --mutex)
            MUTEX_PROFILE=true
            shift
            ;;
        --all-profiles)
            CPU_PROFILE=true
            MEM_PROFILE=true
            BLOCK_PROFILE=true
            MUTEX_PROFILE=true
            shift
            ;;
        --gateway-only)
            RUN_PLUGIN=false
            RUN_STORE=false
            shift
            ;;
        --plugin-only)
            RUN_GATEWAY=false
            RUN_STORE=false
            shift
            ;;
        --store-only)
            RUN_GATEWAY=false
            RUN_PLUGIN=false
            shift
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            usage
            exit 1
            ;;
    esac
done

# Create output directory
mkdir -p "$OUTPUT_DIR"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
RESULT_DIR="$OUTPUT_DIR/$TIMESTAMP"
mkdir -p "$RESULT_DIR"

echo -e "${BLUE}============================================${NC}"
echo -e "${BLUE}   APICerebrus Benchmark Runner${NC}"
echo -e "${BLUE}============================================${NC}"
echo ""
echo -e "${GREEN}Configuration:${NC}"
echo "  Benchmark time: $BENCH_TIME"
echo "  Run count: $BENCH_COUNT"
echo "  Output directory: $RESULT_DIR"
echo "  CPU profiling: $CPU_PROFILE"
echo "  Memory profiling: $MEM_PROFILE"
echo "  Block profiling: $BLOCK_PROFILE"
echo "  Mutex profiling: $MUTEX_PROFILE"
echo ""

# Build benchmark flags
BENCH_FLAGS="-benchtime=$BENCH_TIME -count=$BENCH_COUNT"

if [ "$VERBOSE" = true ]; then
    BENCH_FLAGS="$BENCH_FLAGS -v"
fi

# Build profile flags
PROFILE_FLAGS=""

if [ "$CPU_PROFILE" = true ]; then
    PROFILE_FLAGS="$PROFILE_FLAGS -cpuprofile=$RESULT_DIR/cpu.prof"
fi

if [ "$MEM_PROFILE" = true ]; then
    PROFILE_FLAGS="$PROFILE_FLAGS -memprofile=$RESULT_DIR/mem.prof"
fi

if [ "$BLOCK_PROFILE" = true ]; then
    PROFILE_FLAGS="$PROFILE_FLAGS -blockprofile=$RESULT_DIR/block.prof"
fi

if [ "$MUTEX_PROFILE" = true ]; then
    PROFILE_FLAGS="$PROFILE_FLAGS -mutexprofile=$RESULT_DIR/mutex.prof"
fi

# Function to run benchmarks
run_benchmarks() {
    local package=$1
    local name=$2
    local pattern=$3
    local output_file="$RESULT_DIR/${name}_results.txt"

    echo -e "${YELLOW}Running $name benchmarks...${NC}"

    if [ "$VERBOSE" = true ]; then
        echo "  Package: $package"
        echo "  Pattern: $pattern"
        echo "  Output: $output_file"
    fi

    # Run benchmarks
    if ! go test -bench="$pattern" \
         $BENCH_FLAGS \
         $PROFILE_FLAGS \
         "$package" \
         > "$output_file" 2>&1; then
        echo -e "${RED}  Failed to run $name benchmarks${NC}"
        return 1
    fi

    # Display summary
    echo -e "${GREEN}  ✓ $name benchmarks completed${NC}"

    # Extract and display key metrics
    if [ -f "$output_file" ]; then
        echo ""
        echo "  Summary:"
        grep -E "^(Benchmark|PASS|FAIL|ok|FAIL)" "$output_file" | head -20 | sed 's/^/    /'
        echo ""
    fi

    return 0
}

# Run gateway benchmarks
if [ "$RUN_GATEWAY" = true ]; then
    run_benchmarks "./test/benchmark" "gateway" "BenchmarkRouter|BenchmarkProxy|BenchmarkLoadBalancer|BenchmarkWebSocket|BenchmarkGateway"
fi

# Run plugin benchmarks
if [ "$RUN_PLUGIN" = true ]; then
    run_benchmarks "./test/benchmark" "plugin" "BenchmarkJWT|BenchmarkRateLimit|BenchmarkCORS|BenchmarkCache|BenchmarkPipeline"
fi

# Run store benchmarks
if [ "$RUN_STORE" = true ]; then
    run_benchmarks "./test/benchmark" "store" "BenchmarkUser|BenchmarkAPIKey|BenchmarkBatch|BenchmarkTransaction|BenchmarkConcurrent|BenchmarkSearch|BenchmarkRaw|BenchmarkLarge"
fi

# Generate combined report
echo -e "${BLUE}============================================${NC}"
echo -e "${BLUE}   Generating Report${NC}"
echo -e "${BLUE}============================================${NC}"
echo ""

REPORT_FILE="$RESULT_DIR/report.txt"

cat > "$REPORT_FILE" << EOF
APICerebrus Benchmark Report
============================
Generated: $(date)
Benchmark Time: $BENCH_TIME
Run Count: $BENCH_COUNT

Configuration:
  CPU Profiling: $CPU_PROFILE
  Memory Profiling: $MEM_PROFILE
  Block Profiling: $BLOCK_PROFILE
  Mutex Profiling: $MUTEX_PROFILE

Results:
========

EOF

# Append all results to report
for result_file in "$RESULT_DIR"/*_results.txt; do
    if [ -f "$result_file" ]; then
        echo "$(basename "$result_file" .txt):" >> "$REPORT_FILE"
        echo "----------------------------------------" >> "$REPORT_FILE"
        cat "$result_file" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
    fi
done

# Generate comparison data if multiple runs
if [ "$BENCH_COUNT" -gt 1 ]; then
    echo "Statistical Analysis:" >> "$REPORT_FILE"
    echo "--------------------" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
    echo "Multiple runs detected. Check individual result files for variance." >> "$REPORT_FILE"
fi

echo -e "${GREEN}Report generated: $REPORT_FILE${NC}"
echo ""

# Profile analysis hints
if [ "$CPU_PROFILE" = true ] || [ "$MEM_PROFILE" = true ]; then
    echo -e "${BLUE}Profile Analysis:${NC}"
    echo ""

    if [ "$CPU_PROFILE" = true ] && [ -f "$RESULT_DIR/cpu.prof" ]; then
        echo "  CPU Profile:"
        echo "    File: $RESULT_DIR/cpu.prof"
        echo "    Analyze with: go tool pprof $RESULT_DIR/cpu.prof"
        echo "    Or: go tool pprof -http=:8080 $RESULT_DIR/cpu.prof"
        echo ""
    fi

    if [ "$MEM_PROFILE" = true ] && [ -f "$RESULT_DIR/mem.prof" ]; then
        echo "  Memory Profile:"
        echo "    File: $RESULT_DIR/mem.prof"
        echo "    Analyze with: go tool pprof $RESULT_DIR/mem.prof"
        echo ""
    fi

    if [ "$BLOCK_PROFILE" = true ] && [ -f "$RESULT_DIR/block.prof" ]; then
        echo "  Block Profile:"
        echo "    File: $RESULT_DIR/block.prof"
        echo "    Analyze with: go tool pprof $RESULT_DIR/block.prof"
        echo ""
    fi

    if [ "$MUTEX_PROFILE" = true ] && [ -f "$RESULT_DIR/mutex.prof" ]; then
        echo "  Mutex Profile:"
        echo "    File: $RESULT_DIR/mutex.prof"
        echo "    Analyze with: go tool pprof $RESULT_DIR/mutex.prof"
        echo ""
    fi
fi

# Quick summary
echo -e "${BLUE}============================================${NC}"
echo -e "${BLUE}   Quick Summary${NC}"
echo -e "${BLUE}============================================${NC}"
echo ""

# Extract top benchmarks from results
for result_file in "$RESULT_DIR"/*_results.txt; do
    if [ -f "$result_file" ]; then
        echo -e "${YELLOW}$(basename "$result_file" _results.txt | tr '[:lower:]' '[:upper:]'):${NC}"
        grep "^Benchmark" "$result_file" | head -10 | awk '{printf "  %-50s %15s\n", $1, $3}'
        echo ""
    fi
done

echo -e "${GREEN}All benchmarks completed!${NC}"
echo ""
echo "Results saved to: $RESULT_DIR"
echo ""
echo "To compare with previous runs:"
echo "  benchcmp old.txt new.txt"
echo ""
echo "To view detailed results:"
echo "  cat $REPORT_FILE"
