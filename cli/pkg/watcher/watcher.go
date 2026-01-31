package watcher

import (
	"context"
	"fmt"
	"github.com/ente-io/cli/internal/api"
	"github.com/ente-io/cli/pkg/model"
	"github.com/ente-io/cli/pkg/secrets"
	"github.com/ente-io/cli/pkg/uploader"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Watcher orchestrates folder watching and file uploads
type Watcher struct {
	ctx               context.Context
	client            *api.Client
	storage           Storage
	keyHolder         *secrets.KeyHolder
	state             *model.WatchState
	fileWatcher       *FileWatcher
	debounceQueue     *DebounceQueue
	duplicateHandler  *DuplicateHandler
	uploader          *uploader.Uploader
	uploadWorkers     *sync.WaitGroup
	shutdownChan      chan struct{}
	albumCache        map[string]*AlbumInfo // albumName -> AlbumInfo
	albumCacheMu      sync.RWMutex
	processingFiles   map[string]bool       // Track files currently being processed
	processingFilesMu sync.Mutex
}

// AlbumInfo caches album ID and key
type AlbumInfo struct {
	ID  int64
	Key []byte
}

// NewWatcher creates a new Watcher instance
func NewWatcher(ctx context.Context, client *api.Client, storage Storage, keyHolder *secrets.KeyHolder, state *model.WatchState, config model.UploadConfig) (*Watcher, error) {
	w := &Watcher{
		ctx:              ctx,
		client:           client,
		storage:          storage,
		keyHolder:        keyHolder,
		state:            state,
		uploadWorkers:    &sync.WaitGroup{},
		shutdownChan:     make(chan struct{}),
		albumCache:       make(map[string]*AlbumInfo),
		processingFiles:  make(map[string]bool),
	}

	// Create uploader
	w.uploader = uploader.NewUploader(ctx, client, storage, keyHolder, config)

	// Create duplicate handler
	w.duplicateHandler = NewDuplicateHandler(ctx, client, storage, keyHolder)

	// Create debounce queue
	debounceDuration := time.Duration(state.DebounceMs) * time.Millisecond
	w.debounceQueue = NewDebounceQueue(debounceDuration)

	// Create file watcher
	fileWatcher, err := NewFileWatcher(
		w.onFileEvent,
		w.onNewDirectory,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}
	w.fileWatcher = fileWatcher

	return w, nil
}

// Start begins watching the folder
func (w *Watcher) Start() error {
	// Add root path recursively
	if err := w.fileWatcher.AddRecursive(w.state.WatchPath); err != nil {
		return fmt.Errorf("failed to add watch path: %w", err)
	}

	// Start file watcher
	w.fileWatcher.Start()

	fmt.Printf("Watching folder: %s\n", w.state.WatchPath)
	fmt.Printf("Mode: %s\n", w.state.Mode.String())
	if w.state.Mode == model.WatchModeSpecified {
		fmt.Printf("Album: %s\n", w.state.AlbumName)
	}
	fmt.Printf("Workers: %d\n", w.state.Workers)
	fmt.Printf("Debounce: %dms\n", w.state.DebounceMs)
	fmt.Println("\nPress Ctrl+C to stop watching...")

	return nil
}

// PerformInitialScan scans the folder for existing files and processes them
func (w *Watcher) PerformInitialScan() error {
	fmt.Println("Performing initial scan...")

	var files []string
	err := filepath.Walk(w.state.WatchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible files
		}
		if !info.IsDir() && uploader.IsImageFile(path) {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("initial scan failed: %w", err)
	}

	fmt.Printf("Found %d image(s) in initial scan\n", len(files))

	// Process each file
	for _, file := range files {
		w.processFile(file)
	}

	return nil
}

// onFileEvent is called when a file event is detected (debounce queue callback)
func (w *Watcher) onFileEvent(filePath string) {
	// Add to debounce queue
	w.debounceQueue.Add(filePath, func(path string) {
		w.processFile(path)
	})
}

// onNewDirectory is called when a new directory is created
func (w *Watcher) onNewDirectory(dirPath string) {
	fmt.Printf("New directory detected: %s\n", filepath.Base(dirPath))
}

// processFile processes a single file based on the watch mode
func (w *Watcher) processFile(filePath string) {
	// Check if already processing
	w.processingFilesMu.Lock()
	if w.processingFiles[filePath] {
		w.processingFilesMu.Unlock()
		return
	}
	w.processingFiles[filePath] = true
	w.processingFilesMu.Unlock()

	// Queue for worker processing
	w.uploadWorkers.Add(1)
	go func() {
		defer w.uploadWorkers.Done()
		defer func() {
			w.processingFilesMu.Lock()
			delete(w.processingFiles, filePath)
			w.processingFilesMu.Unlock()
		}()

		var err error
		switch w.state.Mode {
		case model.WatchModeDefault:
			err = w.processFileDefaultMode(filePath)
		case model.WatchModeSpecified:
			err = w.processFileSpecifiedMode(filePath)
		case model.WatchModeFolderAlbums:
			err = w.processFileFolderAlbumsMode(filePath)
		default:
			err = fmt.Errorf("unknown watch mode: %v", w.state.Mode)
		}

		if err != nil {
			fmt.Printf("✗ Failed: %s - %v\n", filepath.Base(filePath), err)
		}

		// Update last processed time
		w.state.LastProcessed = time.Now().Unix()
		if saveErr := w.storage.SaveWatchState(w.ctx, w.state); saveErr != nil {
			fmt.Printf("Warning: failed to save watch state: %v\n", saveErr)
		}
	}()
}

// processFileDefaultMode processes a file in default mode (upload to "CLI Uploads")
func (w *Watcher) processFileDefaultMode(filePath string) error {
	albumName := uploader.DefaultAlbumName
	collectionID, collectionKey, err := w.getOrCreateAlbum(albumName)
	if err != nil {
		return fmt.Errorf("failed to get/create album: %w", err)
	}
	return w.uploadOrAddToAlbum(filePath, collectionID, collectionKey)
}

// processFileSpecifiedMode processes a file in specified mode (upload to user-specified album)
func (w *Watcher) processFileSpecifiedMode(filePath string) error {
	albumName := w.state.AlbumName
	collectionID, collectionKey, err := w.getOrCreateAlbum(albumName)
	if err != nil {
		return fmt.Errorf("failed to get/create album: %w", err)
	}
	return w.uploadOrAddToAlbum(filePath, collectionID, collectionKey)
}

// processFileFolderAlbumsMode processes a file in folder-albums mode (each subfolder becomes an album)
func (w *Watcher) processFileFolderAlbumsMode(filePath string) error {
	// Get relative path
	relPath, err := filepath.Rel(w.state.WatchPath, filePath)
	if err != nil {
		return fmt.Errorf("failed to get relative path: %w", err)
	}

	// Get directory name
	dirName := filepath.Dir(relPath)

	// If file is in root, use default album
	albumName := uploader.DefaultAlbumName
	if dirName != "." && dirName != "" {
		// Use directory name as album name (sanitized)
		albumName = SanitizeAlbumName(dirName)
	}

	collectionID, collectionKey, err := w.getOrCreateAlbum(albumName)
	if err != nil {
		return fmt.Errorf("failed to get/create album: %w", err)
	}
	return w.uploadOrAddToAlbum(filePath, collectionID, collectionKey)
}

// uploadOrAddToAlbum handles uploading a file or adding it to an album if duplicate
func (w *Watcher) uploadOrAddToAlbum(filePath string, targetCollectionID int64, targetCollectionKey []byte) error {
	fileName := filepath.Base(filePath)

	// Compute file hash
	fileHash, err := uploader.ComputeFileHash(filePath)
	if err != nil {
		return fmt.Errorf("failed to compute hash: %w", err)
	}

	// Check for duplicate
	_, collectionID, isDuplicate, err := w.duplicateHandler.CheckAndHandleDuplicate(
		fileHash,
		filePath,
		targetCollectionID,
		targetCollectionKey,
	)

	if err != nil {
		return fmt.Errorf("duplicate check failed: %w", err)
	}

	if isDuplicate {
		if collectionID == targetCollectionID {
			// Already in target album
			fmt.Printf("○ Skipped: %s (already in album)\n", fileName)
		} else {
			// Added to target album
			fmt.Printf("✓ Added to album: %s (duplicate)\n", fileName)
		}
		return nil
	}

	// Not a duplicate - upload the file
	result := w.uploadFile(filePath, targetCollectionID, targetCollectionKey)
	if result.Success {
		fmt.Printf("✓ Uploaded: %s\n", fileName)

		// Save hash mapping
		if err := w.duplicateHandler.SaveFileHashMapping(fileHash, filePath, result.FileID, targetCollectionID); err != nil {
			fmt.Printf("Warning: failed to save hash mapping: %v\n", err)
		}
	} else if result.Skipped {
		fmt.Printf("○ Skipped: %s (duplicate)\n", fileName)
	} else {
		return result.Error
	}

	return nil
}

// uploadFile uploads a single file using the uploader helpers
func (w *Watcher) uploadFile(filePath string, collectionID int64, collectionKey []byte) *uploader.UploadResult {
	result := &uploader.UploadResult{
		FileName: filepath.Base(filePath),
	}

	// Use the uploader's helper to do the actual upload
	// We'll create a helper in the uploader package that accepts collectionID and collectionKey
	fileID, uploadedBytes, err := uploader.UploadSingleFile(
		w.ctx,
		w.client,
		w.storage,
		filePath,
		collectionID,
		collectionKey,
	)

	if err != nil {
		result.Error = err
		return result
	}

	result.Success = true
	result.FileID = fileID
	result.UploadedBytes = uploadedBytes

	return result
}

// getOrCreateAlbum retrieves or creates an album and caches the result
func (w *Watcher) getOrCreateAlbum(albumName string) (int64, []byte, error) {
	// Check cache first
	w.albumCacheMu.RLock()
	if info, exists := w.albumCache[albumName]; exists {
		w.albumCacheMu.RUnlock()
		return info.ID, info.Key, nil
	}
	w.albumCacheMu.RUnlock()

	// Not in cache - fetch or create
	collectionID, collectionKey, err := uploader.GetOrCreateAlbum(w.ctx, w.client, w.keyHolder, albumName, true)
	if err != nil {
		return 0, nil, err
	}

	// Store in cache
	w.albumCacheMu.Lock()
	w.albumCache[albumName] = &AlbumInfo{
		ID:  collectionID,
		Key: collectionKey,
	}
	w.albumCacheMu.Unlock()

	return collectionID, collectionKey, nil
}

// Shutdown gracefully stops the watcher
func (w *Watcher) Shutdown() error {
	fmt.Println("\nShutting down watcher...")

	// Stop file watcher
	w.fileWatcher.Close()

	// Stop debounce queue
	w.debounceQueue.Stop()

	// Wait for pending uploads (with timeout)
	done := make(chan struct{})
	go func() {
		w.uploadWorkers.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Println("All uploads completed")
	case <-time.After(30 * time.Second):
		fmt.Println("Shutdown timeout - some uploads may be incomplete")
	}

	// Save final state
	if err := w.storage.SaveWatchState(w.ctx, w.state); err != nil {
		return fmt.Errorf("failed to save watch state: %w", err)
	}

	return nil
}
