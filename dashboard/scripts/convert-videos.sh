#!/bin/bash
# Convert Playwright video recordings to GIFs for documentation
#
# Usage: ./scripts/convert-videos.sh
#
# Prerequisites:
#   - ffmpeg: brew install ffmpeg (macOS) or apt install ffmpeg (Linux)
#
# This script looks for video.webm files in e2e/test-results/
# and converts them to GIFs in docs/src/assets/screenshots/

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASHBOARD_DIR="$(dirname "$SCRIPT_DIR")"
TEST_RESULTS_DIR="$DASHBOARD_DIR/e2e/test-results"
OUTPUT_DIR="$DASHBOARD_DIR/../docs/src/assets/screenshots"

# Check prerequisites
if ! command -v ffmpeg &> /dev/null; then
    echo "Error: ffmpeg is not installed"
    echo "Install with: brew install ffmpeg (macOS) or apt install ffmpeg (Linux)"
    exit 1
fi

# Create output directory if needed
mkdir -p "$OUTPUT_DIR"

# Check for test results directory
if [ ! -d "$TEST_RESULTS_DIR" ]; then
    echo "No test results found in $TEST_RESULTS_DIR"
    echo "Run 'npm run videos' first to capture videos"
    exit 0
fi

# Check for video capture directories
video_dirs=$(find "$TEST_RESULTS_DIR" -maxdepth 1 -type d -name "*video-capture*" 2>/dev/null || true)
if [ -z "$video_dirs" ]; then
    echo "No video capture directories found"
    echo "Run 'npm run videos' first to capture videos"
    exit 0
fi

echo "Converting videos to GIFs..."
echo "Source: $TEST_RESULTS_DIR"
echo "Output: $OUTPUT_DIR"
echo ""

for video_dir in $video_dirs; do
    if [ -f "$video_dir/video.webm" ]; then
        # Extract test name from directory name
        dir_name=$(basename "$video_dir")
        name=$(echo "$dir_name" | sed 's/.*Animation-Captures-//' | sed 's/-video-capture$//' | tr '-' '_')
        output="$OUTPUT_DIR/$name.gif"

        echo "Converting: $dir_name -> $name.gif"

        # High-quality GIF conversion using ffmpeg
        # - fps=15: 15 frames per second for smooth animation
        # - scale=800:-1: Scale to 800px width, maintain aspect ratio
        # - lanczos: High-quality scaling filter
        # - palettegen/paletteuse: Generate optimal palette for each GIF
        ffmpeg -y -i "$video_dir/video.webm" \
            -vf "fps=15,scale=800:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=256[p];[s1][p]paletteuse=dither=bayer" \
            -loop 0 \
            "$output" 2>/dev/null

        # Show file size
        size=$(ls -lh "$output" | awk '{print $5}')
        echo "  Created: $name.gif ($size)"
    fi
done

echo ""
echo "Conversion complete!"
echo "GIFs are ready in: $OUTPUT_DIR"
