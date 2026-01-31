package uploader

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	thumbnailMaxSize    = 720
	thumbnailQuality    = 75
	thumbnailQualityLow = 60
	thumbnailMaxBytes   = 200 * 1024 // 200KB
)

// CheckFFmpegAvailable checks if ffmpeg is available in PATH
func CheckFFmpegAvailable() error {
	_, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}
	return nil
}

// GenerateThumbnail generates a thumbnail for an image using ffmpeg
func GenerateThumbnail(imagePath string) ([]byte, error) {
	// Create temp directory for thumbnail
	tempDir, err := os.MkdirTemp("", "ente-thumbnail-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	outputPath := filepath.Join(tempDir, "thumbnail.jpg")

	// Try with normal quality first
	err = callFFmpeg(imagePath, outputPath, thumbnailQuality)
	if err != nil {
		return nil, fmt.Errorf("failed to generate thumbnail: %w", err)
	}

	// Check size
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read thumbnail: %w", err)
	}

	// If too large, retry with lower quality
	if len(data) > thumbnailMaxBytes {
		err = callFFmpeg(imagePath, outputPath, thumbnailQualityLow)
		if err != nil {
			return nil, fmt.Errorf("failed to generate thumbnail with lower quality: %w", err)
		}
		data, err = os.ReadFile(outputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read thumbnail: %w", err)
		}
	}

	return data, nil
}

// callFFmpeg calls ffmpeg to generate a thumbnail
func callFFmpeg(inputPath, outputPath string, quality int) error {
	// ffmpeg -i input.jpg -vf "scale=720:720:force_original_aspect_ratio=decrease" -q:v 2 output.jpg
	// -q:v 2 is quality for JPEG (1-31, lower is better)
	// Convert quality percentage to -q:v scale (2-31, where 2=best, 31=worst)
	qScale := 2 + ((100 - quality) * 29 / 100)

	args := []string{
		"-i", inputPath,
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", thumbnailMaxSize, thumbnailMaxSize),
		"-q:v", fmt.Sprintf("%d", qScale),
		"-y", // Overwrite output file
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %w, output: %s", err, string(output))
	}

	return nil
}
