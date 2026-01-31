package cmd

import (
	"fmt"
	"github.com/ente-io/cli/pkg/model"
	"github.com/ente-io/cli/pkg/uploader"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
)

var uploadCmd = &cobra.Command{
	Use:   "upload [files...]",
	Short: "Upload images to Ente Photos",
	Long: `Upload one or more images with end-to-end encryption.

The default album "CLI Uploads" will be created automatically if it doesn't exist.
For custom album names, use --create-album to create them if needed.

Examples:
  ente upload photo.jpg
  ente upload photo1.jpg photo2.jpg photo3.jpg
  ente upload *.jpg --album="Vacation 2024" --create-album
  ente upload photos/ -r --album="Family Photos" --create-album
  ente upload *.png --workers=8`,
	Args: cobra.MinimumNArgs(1),
	Run:  runUpload,
}

func init() {
	rootCmd.AddCommand(uploadCmd)

	uploadCmd.Flags().StringP("album", "a", uploader.DefaultAlbumName, "Album name (default: CLI Uploads)")
	uploadCmd.Flags().BoolP("create-album", "c", false, "Create album if it doesn't exist")
	uploadCmd.Flags().BoolP("recursive", "r", false, "Recursively upload directories")
	uploadCmd.Flags().IntP("workers", "w", uploader.DefaultWorkers, "Number of concurrent uploads")
	uploadCmd.Flags().Bool("force", false, "Force upload even if duplicate exists")
}

func runUpload(cmd *cobra.Command, args []string) {
	// Check ffmpeg availability
	if err := uploader.CheckFFmpegAvailable(); err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Please install ffmpeg and ensure it's in your PATH")
		os.Exit(1)
	}

	// Parse flags
	albumName, _ := cmd.Flags().GetString("album")
	createAlbum, _ := cmd.Flags().GetBool("create-album")
	recursive, _ := cmd.Flags().GetBool("recursive")
	workers, _ := cmd.Flags().GetInt("workers")
	forceUpload, _ := cmd.Flags().GetBool("force")

	// If using the default album name, always create it if it doesn't exist
	if albumName == uploader.DefaultAlbumName {
		createAlbum = true
	}

	// Discover files
	files, err := discoverFiles(args, recursive)
	if err != nil {
		fmt.Printf("Error discovering files: %v\n", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		fmt.Println("No image files found to upload")
		os.Exit(1)
	}

	fmt.Printf("Found %d image(s) to upload\n", len(files))

	// Create upload configuration
	config := model.UploadConfig{
		Workers:      workers,
		ForceUpload:  forceUpload,
		CreateAlbum:  createAlbum,
		ChunkSize:    uploader.DefaultChunkSize,
		MultipartMin: uploader.DefaultMultipartMin,
	}

	// Call the Upload function
	summary, err := ctrl.Upload(files, albumName, config)
	if err != nil {
		fmt.Printf("Upload failed: %v\n", err)
		os.Exit(1)
	}

	// Display summary
	printUploadSummary(summary)

	if summary.FailedFiles > 0 {
		os.Exit(1)
	}
}

// discoverFiles discovers all image files from the provided paths
func discoverFiles(paths []string, recursive bool) ([]string, error) {
	var files []string
	seen := make(map[string]bool)

	for _, path := range paths {
		// Expand glob patterns (e.g., *.jpg)
		matches, err := filepath.Glob(path)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern '%s': %w", path, err)
		}

		// If no matches, treat as literal path
		if len(matches) == 0 {
			matches = []string{path}
		}

		for _, match := range matches {
			if err := collectFiles(match, recursive, &files, seen); err != nil {
				return nil, err
			}
		}
	}

	return files, nil
}

// collectFiles collects image files from a path (file or directory)
func collectFiles(path string, recursive bool, files *[]string, seen map[string]bool) error {
	// Get absolute path to avoid duplicates
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path '%s': %w", path, err)
	}

	// Skip if already processed
	if seen[absPath] {
		return nil
	}
	seen[absPath] = true

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat '%s': %w", path, err)
	}

	if info.IsDir() {
		if !recursive {
			return fmt.Errorf("'%s' is a directory (use -r for recursive upload)", path)
		}

		entries, err := os.ReadDir(absPath)
		if err != nil {
			return fmt.Errorf("failed to read directory '%s': %w", path, err)
		}

		for _, entry := range entries {
			entryPath := filepath.Join(absPath, entry.Name())
			if err := collectFiles(entryPath, recursive, files, seen); err != nil {
				// Skip files/dirs we can't access
				continue
			}
		}
	} else {
		// Check if it's an image file
		if uploader.IsImageFile(absPath) {
			*files = append(*files, absPath)
		}
	}

	return nil
}

// printUploadSummary prints the upload summary
func printUploadSummary(summary *uploader.UploadSummary) {
	fmt.Println("\n=== Upload Summary ===")
	fmt.Printf("Total files: %d\n", summary.TotalFiles)
	fmt.Printf("Completed: %d\n", summary.CompletedFiles)

	if summary.SkippedFiles > 0 {
		fmt.Printf("Skipped (duplicates): %d\n", summary.SkippedFiles)
	}

	if summary.FailedFiles > 0 {
		fmt.Printf("Failed: %d\n", summary.FailedFiles)
		if len(summary.Errors) > 0 {
			fmt.Println("\nErrors:")
			for _, uploadErr := range summary.Errors {
				fmt.Printf("  - %s: %v\n", uploadErr.FileName, uploadErr.Error)
			}
		}
	}

	fmt.Printf("Total uploaded: %s\n", formatBytes(summary.UploadedBytes))
}

// formatBytes formats bytes into human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
