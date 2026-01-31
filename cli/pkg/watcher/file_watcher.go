package watcher

import (
	"fmt"
	"github.com/ente-io/cli/pkg/uploader"
	"github.com/fsnotify/fsnotify"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileWatcher wraps fsnotify.Watcher to provide file system watching capabilities
type FileWatcher struct {
	watcher    *fsnotify.Watcher
	onFile     func(string)      // Callback when a file is ready (after debounce)
	onNewDir   func(string)      // Callback when a new directory is created
	mu         sync.RWMutex      // Protects watched map
	watched    map[string]bool   // Track watched directories
	closed     bool
}

// NewFileWatcher creates a new FileWatcher instance
func NewFileWatcher(onFile func(string), onNewDir func(string)) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	fw := &FileWatcher{
		watcher:  watcher,
		onFile:   onFile,
		onNewDir: onNewDir,
		watched:  make(map[string]bool),
	}

	return fw, nil
}

// AddRecursive adds a directory and all its subdirectories to the watch list
func (fw *FileWatcher) AddRecursive(rootPath string) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip directories we can't access
			return nil
		}
		if info.IsDir() {
			// Skip if already watched
			if fw.watched[path] {
				return nil
			}

			if err := fw.watcher.Add(path); err != nil {
				return fmt.Errorf("failed to watch directory %s: %w", path, err)
			}
			fw.watched[path] = true
		}
		return nil
	})
}

// addDirectory adds a single directory to the watch list (internal, assumes lock held)
func (fw *FileWatcher) addDirectory(dirPath string) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.watched[dirPath] {
		return nil
	}

	if err := fw.watcher.Add(dirPath); err != nil {
		return fmt.Errorf("failed to watch directory %s: %w", dirPath, err)
	}
	fw.watched[dirPath] = true
	return nil
}

// Start begins watching for file system events
func (fw *FileWatcher) Start() {
	go fw.eventLoop()
}

// eventLoop processes file system events
func (fw *FileWatcher) eventLoop() {
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			fw.handleEvent(event)

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fmt.Printf("Watch error: %v\n", err)
		}
	}
}

// handleEvent processes a single file system event
func (fw *FileWatcher) handleEvent(event fsnotify.Event) {
	// We care about: CREATE, WRITE, RENAME (which shows as CREATE)
	if !event.Has(fsnotify.Create) && !event.Has(fsnotify.Write) {
		return
	}

	// Check if it's a directory
	info, err := os.Stat(event.Name)
	if err != nil {
		// File might have been deleted between event and stat
		return
	}

	if info.IsDir() {
		// New directory created - add it to watch list and notify
		if err := fw.addDirectory(event.Name); err != nil {
			fmt.Printf("Failed to watch new directory %s: %v\n", event.Name, err)
			return
		}
		if fw.onNewDir != nil {
			fw.onNewDir(event.Name)
		}
		return
	}

	// It's a file - check if it's an image
	if !uploader.IsImageFile(event.Name) {
		return
	}

	// Call the file callback (will be handled by debounce queue)
	if fw.onFile != nil {
		fw.onFile(event.Name)
	}
}

// Close stops the file watcher
func (fw *FileWatcher) Close() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.closed {
		return nil
	}

	fw.closed = true
	return fw.watcher.Close()
}

// IsImageFile checks if a file has an image extension
// This is a helper that wraps the uploader package function
func IsImageFile(path string) bool {
	return uploader.IsImageFile(path)
}

// SanitizeAlbumName sanitizes a folder name for use as an album name
func SanitizeAlbumName(folderName string) string {
	// Remove leading/trailing whitespace
	name := strings.TrimSpace(folderName)

	// Replace path separators with spaces
	name = strings.ReplaceAll(name, string(filepath.Separator), " ")
	name = strings.ReplaceAll(name, "/", " ")
	name = strings.ReplaceAll(name, "\\", " ")

	// Collapse multiple spaces
	name = strings.Join(strings.Fields(name), " ")

	// If empty, return default
	if name == "" {
		return uploader.DefaultAlbumName
	}

	return name
}
