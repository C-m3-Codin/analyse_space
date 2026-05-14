# Drive Space Analyzer

A lightweight, highly concurrent CLI tool written in Go that analyzes your disk drives to find what's consuming your storage space. Perfect for when your C drive (or any drive) is running out of space and you need to quickly identify the culprits.

## Features

- **Maximum Concurrency**: Uses goroutines with semaphore-based limiting for efficient parallel scanning
- **Memory Efficient**: Designed to use minimal memory even when scanning deep directory structures
- **Tree Visualization**: Beautiful tree view showing directories sorted by size (largest first)
- **File Type Analysis**: Breakdown of storage consumption by file extension
- **Top Files Report**: Lists the 20 largest files found
- **Cross-Platform**: Works on Windows, Linux, and macOS
- **Real-time Memory Stats**: Shows memory usage after scan completes

## Building

```bash
go build -o drive-analyzer .
```

## Usage

### Windows
```bash
./drive-analyzer.exe
```

**Step-by-step:**
1. Run the executable
2. You'll see a list of available drives (e.g., `C:`, `D:`, `E:`)
3. Enter the number corresponding to the drive you want to analyze
4. Wait for the scan to complete (progress shown in real-time)
5. Review the comprehensive report

### Linux/macOS
```bash
./drive-analyzer [path]
```

- **No argument**: Scans the root filesystem (`/`)
- **With path**: Scans the specified directory (e.g., `./drive-analyzer /home/user`)

### Examples

```bash
# Scan C: drive on Windows
./drive-analyzer.exe

# Scan home directory on Linux/Mac
./drive-analyzer /home/username

# Scan current directory
./drive-analyzer .
```

## Output Sections Explained

### 1. Directory Tree
Visual tree structure showing directories and their sizes, sorted largest to smallest.
```
📁 C:\ (45.2 GB)
├── 📁 Users (30.1 GB)
│   ├── 📁 John (28.5 GB)
│   │   ├── 📁 Downloads (15.2 GB)
│   │   └── 📁 AppData (10.1 GB)
```

### 2. File Type Breakdown
Bar chart showing which file types consume the most space with percentages.
```
File Type Analysis:
.mp4    ████████████████████ 45.2% (20.4 GB)
.iso    ████████████ 28.1% (12.7 GB)
.zip    ████████ 15.3% (6.9 GB)
```

### 3. Top Largest Files
List of the 50 biggest files with their full paths and sizes.

### 4. Memory Stats
Shows how much memory was used during the scan to demonstrate efficiency.

## Tips for Finding Space Hogs

1. **Look for large directories** at the top of the tree view
2. **Check file type breakdown** - videos, ISOs, and archives often take the most space
3. **Review top files list** - single large files might be easy targets for deletion
4. **Common culprits**:
   - `Downloads` folder
   - `AppData\Local\Temp` (Windows)
   - Old game installations
   - Video files and disk images
   - Log files that grew too large

## Configuration

You can modify these constants in `main.go` to adjust behavior:

- `depthLimit`: Maximum directory depth to scan (default: 100)
- `semaphore`: Concurrent goroutine limit (default: 30) - increase for faster scans on SSDs
- `maxTopFiles`: Number of top files to track (default: 50)

## Performance Notes

- **Concurrency**: Uses 30 parallel workers by default for optimal performance
- **Memory**: Typically uses <1MB even for large drives
- **Speed**: Scans thousands of files per second depending on disk speed
- **Safe**: Read-only operation, never modifies your files

## License

MIT
