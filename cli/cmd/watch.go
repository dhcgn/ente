package cmd

import (
	"fmt"
	"github.com/ente-io/cli/pkg/model"
	"github.com/ente-io/cli/pkg/uploader"
	"github.com/ente-io/cli/pkg/watcher"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var watchCmd = &cobra.Command{
	Use:   "watch <folder>",
	Short: "Watch a folder and automatically upload new images",
	Long: `Watch a folder for new images and automatically upload them with end-to-end encryption.

The watch command has three modes:

1. Default Mode (no flags): Upload all files to "CLI Uploads" album
   Example: ente watch ~/Photos

2. Specified Album Mode (--album flag): Upload all files to a specific album
   Example: ente watch ~/Photos --album="Vacation 2024"

3. Folder-as-Album Mode (--folder-albums flag): Each subfolder becomes an album
   Example: ente watch ~/Photos --folder-albums

Features:
  - Recursive watching (automatically watches subdirectories)
  - Duplicate detection (files are not re-uploaded)
  - Album assignment (duplicates are added to target album)
  - Debouncing (waits for file writes to complete)
  - State persistence (recovers on restart)
  - Graceful shutdown (Ctrl+C)

Options:
  --album <name>          Upload to specified album (Mode 2)
  --folder-albums         Each subfolder becomes album (Mode 3)
  --workers <n>           Concurrent uploads (default: 4)
  --debounce <ms>         File write debounce in milliseconds (default: 5000)
  --initial-scan          Process existing files on startup

Examples:
  ente watch ~/Photos
  ente watch ~/Photos --album="Family Photos"
  ente watch ~/Photos --folder-albums --initial-scan
  ente watch ~/Photos --workers=8 --debounce=3000`,
	Args: cobra.ExactArgs(1),
	Run:  runWatch,
}

func init() {
	rootCmd.AddCommand(watchCmd)

	watchCmd.Flags().StringP("album", "a", "", "Upload to specified album (Mode 2)")
	watchCmd.Flags().Bool("folder-albums", false, "Each subfolder becomes album (Mode 3)")
	watchCmd.Flags().IntP("workers", "w", uploader.DefaultWorkers, "Number of concurrent uploads")
	watchCmd.Flags().Int("debounce", 5000, "File write debounce in milliseconds")
	watchCmd.Flags().Bool("initial-scan", false, "Process existing files on startup")
}

func runWatch(cmd *cobra.Command, args []string) {
	// Check ffmpeg availability
	if err := uploader.CheckFFmpegAvailable(); err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Please install ffmpeg and ensure it's in your PATH")
		os.Exit(1)
	}

	// Parse flags
	albumName, _ := cmd.Flags().GetString("album")
	folderAlbums, _ := cmd.Flags().GetBool("folder-albums")
	workers, _ := cmd.Flags().GetInt("workers")
	debounceMs, _ := cmd.Flags().GetInt("debounce")
	initialScan, _ := cmd.Flags().GetBool("initial-scan")

	// Validate flags
	if albumName != "" && folderAlbums {
		fmt.Println("Error: --album and --folder-albums are mutually exclusive")
		os.Exit(1)
	}

	// Get absolute path
	watchPath := args[0]
	absPath, err := filepath.Abs(watchPath)
	if err != nil {
		fmt.Printf("Error: invalid path '%s': %v\n", watchPath, err)
		os.Exit(1)
	}

	// Verify path exists and is a directory
	info, err := os.Stat(absPath)
	if err != nil {
		fmt.Printf("Error: path '%s' does not exist: %v\n", absPath, err)
		os.Exit(1)
	}
	if !info.IsDir() {
		fmt.Printf("Error: path '%s' is not a directory\n", absPath)
		os.Exit(1)
	}

	// Determine watch mode
	watchMode := model.WatchModeDefault
	if folderAlbums {
		watchMode = model.WatchModeFolderAlbums
	} else if albumName != "" {
		watchMode = model.WatchModeSpecified
	} else {
		albumName = uploader.DefaultAlbumName
	}

	// Create watch state
	watchState := &model.WatchState{
		WatchPath:     absPath,
		Mode:          watchMode,
		AlbumName:     albumName,
		Workers:       workers,
		DebounceMs:    debounceMs,
		StartedAt:     time.Now().Unix(),
		LastProcessed: time.Now().Unix(),
	}

	// Initialize buckets and get properly configured context
	ctx, err := ctrl.InitializeWatchBuckets(cmd.Context())
	if err != nil {
		fmt.Printf("Error: failed to initialize buckets: %v\n", err)
		os.Exit(1)
	}

	// Create upload configuration
	uploadConfig := model.UploadConfig{
		Workers:      workers,
		ForceUpload:  false,
		CreateAlbum:  true,
		ChunkSize:    uploader.DefaultChunkSize,
		MultipartMin: uploader.DefaultMultipartMin,
	}

	// Create watcher
	w, err := watcher.NewWatcher(
		ctx,
		ctrl.Client,
		ctrl,
		ctrl.KeyHolder,
		watchState,
		uploadConfig,
	)
	if err != nil {
		fmt.Printf("Error: failed to create watcher: %v\n", err)
		os.Exit(1)
	}

	// Perform initial scan if requested
	if initialScan {
		if err := w.PerformInitialScan(); err != nil {
			fmt.Printf("Error: initial scan failed: %v\n", err)
			os.Exit(1)
		}
	}

	// Start watching
	if err := w.Start(); err != nil {
		fmt.Printf("Error: failed to start watcher: %v\n", err)
		os.Exit(1)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for shutdown signal
	<-sigChan

	// Graceful shutdown
	if err := w.Shutdown(); err != nil {
		fmt.Printf("Warning: shutdown error: %v\n", err)
	}

	fmt.Println("Watch stopped")
}
