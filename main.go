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
