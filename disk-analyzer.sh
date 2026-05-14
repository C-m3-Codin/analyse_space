#!/bin/bash

# Disk Usage Analyzer Script
# Generates a detailed report of top 10 directories and files by size

OUTPUT_FILE="${1:-disk_usage_report.txt}"
TARGET_DIR="${2:-.}"

echo "========================================" > "$OUTPUT_FILE"
echo "       DISK USAGE ANALYSIS REPORT      " >> "$OUTPUT_FILE"
echo "========================================" >> "$OUTPUT_FILE"
echo "Generated: $(date)" >> "$OUTPUT_FILE"
echo "Target Directory: $(realpath "$TARGET_DIR")" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# Top 10 Directories by Size (first-level only, excluding root)
echo "========================================" >> "$OUTPUT_FILE"
echo "   TOP 10 DIRECTORIES BY SIZE          " >> "$OUTPUT_FILE"
echo "========================================" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

for dir in "$TARGET_DIR"/*/ "$TARGET_DIR"/.[!.]*; do
    if [ -d "$dir" ]; then
        du -sh "$dir" 2>/dev/null
    fi
done | sort -rh | head -10 | while read size dir; do
    printf "%-10s %s\n" "$size" "$dir" >> "$OUTPUT_FILE"
done

echo "" >> "$OUTPUT_FILE"

# Detailed breakdown for each top directory
echo "========================================" >> "$OUTPUT_FILE"
echo "   DETAILED BREAKDOWN OF TOP DIRECTORIES" >> "$OUTPUT_FILE"
echo "========================================" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

# Store top directories in temp file to avoid subshell issues
TMPDIR=$(mktemp -d)
TOPDIRS="$TMPDIR/topdirs.txt"

for dir in "$TARGET_DIR"/*/ "$TARGET_DIR"/.[!.]*; do
    if [ -d "$dir" ]; then
        du -sh "$dir" 2>/dev/null
    fi
done | sort -rh | head -10 > "$TOPDIRS"

while read size dir; do
    echo "Directory: $dir ($size)" >> "$OUTPUT_FILE"
    echo "----------------------------------------" >> "$OUTPUT_FILE"
    # Show contents of this directory (files and subdirectories)
    find "$dir" -maxdepth 1 -mindepth 1 -exec du -sh {} + 2>/dev/null | sort -rh | head -10 | while read subsize subitem; do
        printf "  %-10s %s\n" "$subsize" "$subitem" >> "$OUTPUT_FILE"
    done
    echo "" >> "$OUTPUT_FILE"
done < "$TOPDIRS"

rm -rf "$TMPDIR"

# Top 10 Files by Size
echo "========================================" >> "$OUTPUT_FILE"
echo "   TOP 10 FILES BY SIZE                " >> "$OUTPUT_FILE"
echo "========================================" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

find "$TARGET_DIR" -type f -exec du -h {} + 2>/dev/null | sort -rh | head -10 | while read size file; do
    printf "%-10s %s\n" "$size" "$file" >> "$OUTPUT_FILE"
done

echo "" >> "$OUTPUT_FILE"

# Summary Statistics
echo "========================================" >> "$OUTPUT_FILE"
echo "   SUMMARY STATISTICS                  " >> "$OUTPUT_FILE"
echo "========================================" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"
echo "Total disk usage in target directory:" >> "$OUTPUT_FILE"
du -sh "$TARGET_DIR" >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"
echo "Total number of files:" >> "$OUTPUT_FILE"
find "$TARGET_DIR" -type f | wc -l >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"
echo "Total number of directories:" >> "$OUTPUT_FILE"
find "$TARGET_DIR" -type d | wc -l >> "$OUTPUT_FILE"
echo "" >> "$OUTPUT_FILE"

echo "========================================" >> "$OUTPUT_FILE"
echo "         END OF REPORT                 " >> "$OUTPUT_FILE"
echo "========================================" >> "$OUTPUT_FILE"

echo "Report generated successfully: $(realpath "$OUTPUT_FILE")"
echo ""
echo "=== QUICK SUMMARY (Terminal Output) ==="
echo ""
echo "Top 10 Directories:"
for dir in "$TARGET_DIR"/*/ "$TARGET_DIR"/.[!.]*; do
    if [ -d "$dir" ]; then
        du -sh "$dir" 2>/dev/null
    fi
done | sort -rh | head -10
echo ""
echo "Top 10 Files:"
find "$TARGET_DIR" -type f -exec du -h {} + 2>/dev/null | sort -rh | head -10
echo ""
echo "Full report saved to: $(realpath "$OUTPUT_FILE")"
