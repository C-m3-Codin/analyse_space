package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DirBreakdown holds breakdown information for a directory
type DirBreakdown struct {
	Name     string
	Path     string
	Size     int64
	Items    []BreakdownItem
}

// BreakdownItem represents a file or subdirectory in a breakdown
type BreakdownItem struct {
	Name  string
	Size  int64
	IsDir bool
}

// Node represents a directory or file in the tree
type Node struct {
	Name     string
	Path     string
	Size     int64
	IsDir    bool
	Children []*Node
	FileType string // extension for files
}

// DriveInfo holds information about a drive
type DriveInfo struct {
	Letter       string
	Label        string
	TotalSpace   uint64
	FreeSpace    uint64
	UsedSpace    uint64
	UsagePercent float64
}

var (
	nodeCount      int64
	depthLimit     = 100                     // Safety limit
	semaphore      = make(chan struct{}, 30) // Limit concurrent goroutines
	fileTypeStats  = make(map[string]int64)
	fileTypeMutex  sync.Mutex
	topFiles       = make([]*Node, 0)
	topFilesMutex  sync.Mutex
	maxTopFiles    = 50
)

func main() {
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║           DRIVE SPACE ANALYZER - Memory Efficient         ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()

	runtime.GOMAXPROCS(runtime.NumCPU())

	// Detect OS and handle accordingly
	if isWindows() {
		drives := getDrives()

		if len(drives) == 0 {
			fmt.Println("No drives found!")
			return
		}

		fmt.Println("Available Drives:")
		fmt.Println("─────────────────────────────────────────────────────────")
		for i, d := range drives {
			fmt.Printf("[%d] %s: ", i+1, d.Letter)
			if d.TotalSpace > 0 {
				fmt.Printf("Total: %.2f GB | ", float64(d.TotalSpace)/1e9)
				fmt.Printf("Free: %.2f GB | ", float64(d.FreeSpace)/1e9)
				fmt.Printf("Used: %.2f GB (%.1f%%)\n", float64(d.UsedSpace)/1e9, d.UsagePercent)
			} else {
				fmt.Println("(Size info not available)")
			}
			if d.Label != "" {
				fmt.Printf("    Label: %s\n", d.Label)
			}
		}
		fmt.Println("─────────────────────────────────────────────────────────")

		fmt.Print("\nSelect drive number to analyze (or 'q' to quit): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "q" || input == "Q" {
			return
		}

		var selected int
		fmt.Sscanf(input, "%d", &selected)

		if selected < 1 || selected > len(drives) {
			fmt.Println("Invalid selection!")
			return
		}

		selectedDrive := drives[selected-1]
		rootPath := selectedDrive.Letter + ":/"
		fmt.Printf("\nAnalyzing %s... Please wait.\n", rootPath)
		fmt.Println("Scanning directory structure with maximum concurrency...\n")

		analyzeDrive(rootPath, selectedDrive.Letter)

	} else {
		// Linux/Mac mode
		fmt.Println("Running in Linux/Mac mode...")
		fmt.Println("\nAnalyzing root filesystem...")
		analyzeDrive("/", "root")
	}
}

func isWindows() bool {
	return os.PathSeparator == '\\'
}

func getDrives() []DriveInfo {
	var drives []DriveInfo

	if isWindows() {
		driveLetters := []string{"C", "D", "E", "F", "G", "H"}
		for _, letter := range driveLetters {
			path := letter + `:/`
			if _, err := os.Stat(path); err == nil {
				drives = append(drives, DriveInfo{
					Letter: letter,
					Label:  "",
				})
			}
		}
		return drives
	}

	// For Linux/Mac, just return root
	return []DriveInfo{{Letter: "/", Label: "Root"}}
}

func analyzeDrive(rootPath, driveName string) {
	startTime := time.Now()

	root := &Node{
		Name:  driveName,
		Path:  rootPath,
		IsDir: true,
	}

	// Use WaitGroup for proper synchronization
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		processDirectory(root, 0, &wg)
	}()

	wg.Wait()

	elapsed := time.Since(startTime)

	var totalSize int64
	calculateTotalSize(root, &totalSize)

	fmt.Println("\n" + strings.Repeat("═", 70))
	fmt.Printf("SCAN COMPLETE - %s\n", elapsed.Round(time.Millisecond))
	fmt.Println(strings.Repeat("═", 70))
	fmt.Printf("Total Nodes Scanned: %d\n", atomic.LoadInt64(&nodeCount))
	fmt.Printf("Total Size: %s\n", formatSize(totalSize))
	fmt.Println()

	// Sort children by size
	sort.Slice(root.Children, func(i, j int) bool {
		return root.Children[i].Size > root.Children[j].Size
	})

	// Display tree view
	fmt.Println("\n📁 DIRECTORY TREE (sorted by size - largest first)")
	fmt.Println(strings.Repeat("─", 70))
	displayTree(root, "", true, 10)

	// Display file type analysis
	fmt.Println("\n\n📊 FILE TYPE BREAKDOWN")
	fmt.Println(strings.Repeat("─", 70))
	displayFileTypeStats()

	// Display top largest files
	fmt.Println("\n\n🔥 TOP LARGEST FILES")
	fmt.Println(strings.Repeat("─", 70))
	displayTopFiles()

	// Memory stats
	runtime.GC()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	fmt.Println("\n\n⚡ MEMORY USAGE STATS")
	fmt.Println(strings.Repeat("─", 70))
	fmt.Printf("Alloc: %s\n", formatSize(int64(memStats.Alloc)))
	fmt.Printf("TotalAlloc: %s\n", formatSize(int64(memStats.TotalAlloc)))
	fmt.Printf("Sys: %s\n", formatSize(int64(memStats.Sys)))
	fmt.Printf("NumGC: %d\n", memStats.NumGC)

	// Generate HTML report
	fmt.Println("\n\n🌐 Generating HTML Report...")
	generateHTMLReport(root, driveName, totalSize, atomic.LoadInt64(&nodeCount), elapsed)
}

func processDirectory(node *Node, depth int, parentWg *sync.WaitGroup) {
	if depth > depthLimit {
		return
	}

	atomic.AddInt64(&nodeCount, 1)

	entries, err := os.ReadDir(node.Path)
	if err != nil {
		return
	}

	children := make([]*Node, 0)
	var childWg sync.WaitGroup

	for _, entry := range entries {
		childPath := filepath.Join(node.Path, entry.Name())

		if shouldSkip(childPath, entry) {
			continue
		}

		childNode := &Node{
			Name:  entry.Name(),
			Path:  childPath,
			IsDir: entry.IsDir(),
		}

		if entry.IsDir() {
			// Acquire semaphore before spawning goroutine
			select {
			case semaphore <- struct{}{}:
				childWg.Add(1)
				go func(cn *Node, d int) {
					defer func() {
						<-semaphore
						childWg.Done()
					}()
					processDirectory(cn, d+1, parentWg)
				}(childNode, depth)
			default:
				// Semaphore full, process synchronously
				processDirectory(childNode, depth+1, parentWg)
			}
		} else {
			info, err := entry.Info()
			if err == nil {
				childNode.Size = info.Size()
				childNode.FileType = strings.ToLower(filepath.Ext(entry.Name()))

				fileTypeMutex.Lock()
				fileTypeStats[childNode.FileType] += childNode.Size
				fileTypeMutex.Unlock()

				trackTopFile(childNode)
			}
		}

		children = append(children, childNode)
	}

	// Wait for all child directory goroutines to complete
	childWg.Wait()

	// Calculate total size from children
	var dirSize int64
	for _, child := range children {
		dirSize += child.Size
	}
	node.Size = dirSize
	node.Children = children
}

func shouldSkip(path string, entry fs.DirEntry) bool {
	name := entry.Name()

	skipDirs := map[string]bool{
		"$Recycle.Bin":          true,
		"System Volume Information": true,
		"pagefile.sys":          true,
		"hiberfil.sys":          true,
		"swapfile.sys":          true,
		"/proc":                 true,
		"/sys":                  true,
		"/dev":                  true,
	}

	if skipDirs[name] {
		return true
	}

	if entry.Type()&os.ModeSymlink != 0 {
		return true
	}

	return false
}

func trackTopFile(node *Node) {
	topFilesMutex.Lock()
	defer topFilesMutex.Unlock()

	topFiles = append(topFiles, node)
	sort.Slice(topFiles, func(i, j int) bool {
		return topFiles[i].Size > topFiles[j].Size
	})

	if len(topFiles) > maxTopFiles {
		topFiles = topFiles[:maxTopFiles]
	}
}

func calculateTotalSize(node *Node, total *int64) {
	*total += node.Size
	for _, child := range node.Children {
		calculateTotalSize(child, total)
	}
}

func displayTree(node *Node, prefix string, isTail bool, maxDepth int) {
	if maxDepth <= 0 {
		fmt.Printf("%s... (depth limit reached)\n", prefix)
		return
	}

	marker := "├── "
	if isTail {
		marker = "└── "
	}

	sizeStr := formatSize(node.Size)

	if node.IsDir {
		fmt.Printf("%s%s 📂 %s (%s)\n", prefix, marker, node.Name, sizeStr)
	} else {
		fmt.Printf("%s%s 📄 %s (%s)\n", prefix, marker, node.Name, sizeStr)
	}

	sort.Slice(node.Children, func(i, j int) bool {
		return node.Children[i].Size > node.Children[j].Size
	})

	newPrefix := prefix
	if isTail {
		newPrefix += "    "
	} else {
		newPrefix += "│   "
	}

	displayCount := len(node.Children)
	if displayCount > 20 && node.Size > 100*1024*1024 {
		displayCount = 20
	}

	for i, child := range node.Children {
		isLast := i == len(node.Children)-1 || i >= displayCount-1
		if i < displayCount {
			displayTree(child, newPrefix, isLast, maxDepth-1)
		} else if i == displayCount {
			fmt.Printf("%s    ... (%d more items)\n", newPrefix, len(node.Children)-displayCount)
			break
		}
	}
}

func displayFileTypeStats() {
	type fileTypeStat struct {
		ext  string
		size int64
	}

	var stats []fileTypeStat
	fileTypeMutex.Lock()
	for ext, size := range fileTypeStats {
		stats = append(stats, fileTypeStat{ext, size})
	}
	fileTypeMutex.Unlock()

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].size > stats[j].size
	})

	total := int64(0)
	for _, s := range stats {
		total += s.size
	}

	for i, s := range stats {
		if i >= 25 {
			break
		}
		percent := float64(s.size) / float64(total) * 100
		ext := s.ext
		if ext == "" {
			ext = "(no extension)"
		}
		bar := strings.Repeat("█", int(percent/2))
		fmt.Printf("%-15s %s %s (%.1f%%)\n", ext, bar, formatSize(s.size), percent)
	}
}

func displayTopFiles() {
	topFilesMutex.Lock()
	defer topFilesMutex.Unlock()

	for i, f := range topFiles {
		if i >= 20 {
			break
		}
		fmt.Printf("%2d. %-60s %s\n", i+1, truncatePath(f.Path, 60), formatSize(f.Size))
	}
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	if maxLen < 3 {
		return path[:maxLen]
	}
	return "..." + path[len(path)-maxLen+3:]
}

// generateHTMLReport creates an interactive HTML report
func generateHTMLReport(root *Node, driveName string, totalSize int64, nodeCount int64, elapsed time.Duration) {
	filename := "disk_usage_report.html"
	
	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creating HTML file: %v\n", err)
		return
	}
	defer file.Close()

	// Collect top directories
	topDirs := collectTopDirectories(root, 10)

	// Collect file type stats
	fileTypeData := collectFileTypeStats()

	// Collect top files
	topFilesData := collectTopFilesData(20)

	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Disk Usage Report - ` + driveName + `</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { 
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; 
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        .container { 
            max-width: 1400px; 
            margin: 0 auto; 
            background: white;
            border-radius: 15px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            overflow: hidden;
        }
        .header {
            background: linear-gradient(135deg, #2c3e50 0%, #3498db 100%);
            color: white;
            padding: 30px;
            text-align: center;
        }
        .header h1 { font-size: 2.5em; margin-bottom: 10px; }
        .header p { opacity: 0.9; font-size: 1.1em; }
        .summary-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            padding: 30px;
            background: #f8f9fa;
        }
        .summary-card {
            background: white;
            padding: 20px;
            border-radius: 10px;
            box-shadow: 0 5px 15px rgba(0,0,0,0.1);
            text-align: center;
            border-left: 4px solid #3498db;
        }
        .summary-card h3 { color: #7f8c8d; font-size: 0.9em; margin-bottom: 10px; }
        .summary-card .value { font-size: 1.8em; font-weight: bold; color: #2c3e50; }
        .section { padding: 30px; border-bottom: 1px solid #eee; }
        .section h2 { 
            color: #2c3e50; 
            margin-bottom: 20px; 
            padding-bottom: 10px;
            border-bottom: 3px solid #3498db;
            display: inline-block;
        }
        table { 
            width: 100%; 
            border-collapse: collapse; 
            margin-top: 15px;
        }
        th, td { 
            padding: 12px 15px; 
            text-align: left; 
            border-bottom: 1px solid #ddd;
        }
        th { 
            background: #3498db; 
            color: white; 
            font-weight: 600;
        }
        tr:hover { background: #f5f6fa; }
        .size-col { font-family: monospace; color: #e74c3c; font-weight: bold; }
        .bar-container { 
            width: 100%; 
            background: #ecf0f1; 
            border-radius: 5px; 
            height: 20px;
            overflow: hidden;
        }
        .bar { 
            height: 100%; 
            background: linear-gradient(90deg, #3498db, #2ecc71);
            transition: width 0.3s;
        }
        .breakdown { 
            margin-top: 15px; 
            padding: 15px; 
            background: #f8f9fa;
            border-radius: 8px;
        }
        .breakdown h4 { color: #7f8c8d; margin-bottom: 10px; }
        .breakdown-item {
            display: flex;
            justify-content: space-between;
            padding: 8px 0;
            border-bottom: 1px dashed #ddd;
        }
        .breakdown-item:last-child { border-bottom: none; }
        .icon { margin-right: 8px; }
        .toggle-btn {
            background: #3498db;
            color: white;
            border: none;
            padding: 8px 15px;
            border-radius: 5px;
            cursor: pointer;
            font-size: 0.9em;
            margin-top: 10px;
        }
        .toggle-btn:hover { background: #2980b9; }
        .hidden { display: none; }
        .file-type-bar { display: flex; align-items: center; gap: 10px; }
        .file-type-name { min-width: 150px; font-weight: 600; }
        .progress-bg { 
            flex: 1; 
            background: #ecf0f1; 
            border-radius: 5px; 
            height: 25px;
            overflow: hidden;
        }
        .progress-fill { 
            height: 100%; 
            background: linear-gradient(90deg, #667eea, #764ba2);
            border-radius: 5px;
            display: flex;
            align-items: center;
            justify-content: flex-end;
            padding-right: 10px;
            color: white;
            font-size: 0.85em;
            font-weight: bold;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>📊 Disk Usage Analysis Report</h1>
            <p>Drive: ` + driveName + ` | Generated: ` + time.Now().Format("2006-01-02 15:04:05") + `</p>
        </div>

        <div class="summary-grid">
            <div class="summary-card">
                <h3>📁 Total Size</h3>
                <div class="value">` + formatSize(totalSize) + `</div>
            </div>
            <div class="summary-card">
                <h3>🔍 Nodes Scanned</h3>
                <div class="value">` + fmt.Sprintf("%d", nodeCount) + `</div>
            </div>
            <div class="summary-card">
                <h3>⏱️ Scan Time</h3>
                <div class="value">` + elapsed.Round(time.Millisecond).String() + `</div>
            </div>
            <div class="summary-card">
                <h3>📄 Top Files</h3>
                <div class="value">` + fmt.Sprintf("%d", len(topFilesData)) + `</div>
            </div>
        </div>

        <div class="section">
            <h2>🏆 Top 10 Directories by Size</h2>
            <table>
                <thead>
                    <tr>
                        <th>#</th>
                        <th>Directory</th>
                        <th>Size</th>
                        <th>Details</th>
                    </tr>
                </thead>
                <tbody>`

	for i, dir := range topDirs {
		html += fmt.Sprintf(`
                    <tr>
                        <td>%d</td>
                        <td><span class="icon">📂</span>%s</td>
                        <td class="size-col">%s</td>
                        <td><button class="toggle-btn" onclick="toggleBreakdown('breakdown-%d')">View Breakdown</button></td>
                    </tr>`, i+1, dir.Name, formatSize(dir.Size), i)

		html += fmt.Sprintf(`
                    <tr id="breakdown-%d" class="hidden">
                        <td colspan="4">
                            <div class="breakdown">
                                <h4>Contents of %s:</h4>`, dir.Name)

		for j, item := range dir.Items {
			if j >= 10 {
				break
			}
			icon := "📄"
			if item.IsDir {
				icon = "📁"
			}
			html += fmt.Sprintf(`
                                <div class="breakdown-item">
                                    <span><span class="icon">%s</span>%s</span>
                                    <span class="size-col">%s</span>
                                </div>`, icon, item.Name, formatSize(item.Size))
		}
		if len(dir.Items) > 10 {
			html += fmt.Sprintf(`<div style="text-align:center;padding:10px;color:#7f8c8d;">... and %d more items</div>`, len(dir.Items)-10)
		}

		html += `
                            </div>
                        </td>
                    </tr>`
	}

	html += `
                </tbody>
            </table>
        </div>

        <div class="section">
            <h2>📈 File Type Distribution</h2>`

	totalForPercent := int64(0)
	for _, ft := range fileTypeData {
		totalForPercent += ft.Size
	}

	for i, ft := range fileTypeData {
		if i >= 15 {
			break
		}
		percent := float64(0)
		if totalForPercent > 0 {
			percent = float64(ft.Size) / float64(totalForPercent) * 100
		}
		ext := ft.Extension
		if ext == "" {
			ext = "(no extension)"
		}
		html += fmt.Sprintf(`
            <div class="file-type-bar">
                <span class="file-type-name">%s</span>
                <div class="progress-bg">
                    <div class="progress-fill" style="width: %.1f%%;">%.1f%%</div>
                </div>
                <span style="min-width:100px;text-align:right;font-weight:bold;">%s</span>
            </div>`, ext, percent, percent, formatSize(ft.Size))
	}

	html += `
        </div>

        <div class="section">
            <h2>🔥 Top 20 Largest Files</h2>
            <table>
                <thead>
                    <tr>
                        <th>#</th>
                        <th>File Path</th>
                        <th>Size</th>
                    </tr>
                </thead>
                <tbody>`

	for i, f := range topFilesData {
		html += fmt.Sprintf(`
                    <tr>
                        <td>%d</td>
                        <td style="font-family:monospace;font-size:0.9em;">%s</td>
                        <td class="size-col">%s</td>
                    </tr>`, i+1, f.Path, formatSize(f.Size))
	}

	html += `
                </tbody>
            </table>
        </div>
    </div>

    <script>
        function toggleBreakdown(id) {
            var el = document.getElementById(id);
            if (el.classList.contains('hidden')) {
                el.classList.remove('hidden');
            } else {
                el.classList.add('hidden');
            }
        }
    </script>
</body>
</html>`

	_, err = file.WriteString(html)
	if err != nil {
		fmt.Printf("Error writing HTML file: %v\n", err)
		return
	}

	fmt.Printf("✅ HTML report saved to: %s\n", filename)
}

// FileTypeStat holds file type statistics
type FileTypeStat struct {
	Extension string
	Size      int64
}

func collectFileTypeStats() []FileTypeStat {
	type fileTypeStat struct {
		ext  string
		size int64
	}

	var stats []fileTypeStat
	fileTypeMutex.Lock()
	for ext, size := range fileTypeStats {
		stats = append(stats, fileTypeStat{ext, size})
	}
	fileTypeMutex.Unlock()

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].size > stats[j].size
	})

	result := make([]FileTypeStat, len(stats))
	for i, s := range stats {
		result[i] = FileTypeStat{Extension: s.ext, Size: s.size}
	}
	return result
}

func collectTopDirectories(root *Node, limit int) []DirBreakdown {
	var dirs []DirBreakdown

	var collect func(node *Node)
	collect = func(node *Node) {
		if node.IsDir && node.Size > 0 {
			items := make([]BreakdownItem, 0)
			for _, child := range node.Children {
				items = append(items, BreakdownItem{
					Name:  child.Name,
					Size:  child.Size,
					IsDir: child.IsDir,
				})
			}
			sort.Slice(items, func(i, j int) bool {
				return items[i].Size > items[j].Size
			})

			dirs = append(dirs, DirBreakdown{
				Name:  node.Name,
				Path:  node.Path,
				Size:  node.Size,
				Items: items,
			})
		}
		for _, child := range node.Children {
			if child.IsDir {
				collect(child)
			}
		}
	}

	collect(root)

	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Size > dirs[j].Size
	})

	if len(dirs) > limit {
		dirs = dirs[:limit]
	}

	return dirs
}

func collectTopFilesData(limit int) []struct {
	Path string
	Size int64
} {
	topFilesMutex.Lock()
	defer topFilesMutex.Unlock()

	result := make([]struct {
		Path string
		Size int64
	}, 0)

	count := limit
	if len(topFiles) < limit {
		count = len(topFiles)
	}

	for i := 0; i < count; i++ {
		result = append(result, struct {
			Path string
			Size int64
		}{
			Path: topFiles[i].Path,
			Size: topFiles[i].Size,
		})
	}

	return result
}
