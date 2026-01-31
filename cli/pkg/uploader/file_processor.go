package uploader

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/ente-io/cli/pkg/model"
	"github.com/rwcarlsen/goexif/exif"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Supported image extensions
var supportedImageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".bmp":  true,
	".webp": true,
	".heic": true,
	".heif": true,
	".tiff": true,
	".tif":  true,
}

// ComputeFileHash computes SHA256 hash of file content
func ComputeFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to compute hash: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ValidateImageFile checks if the file exists, is readable, and has a valid image extension
func ValidateImageFile(filePath string) error {
	// Check file exists
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", filePath)
		}
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Check it's not a directory
	if info.IsDir() {
		return fmt.Errorf("path is a directory: %s", filePath)
	}

	// Check readable
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("file not readable: %w", err)
	}
	file.Close()

	// Check extension
	ext := strings.ToLower(filepath.Ext(filePath))
	if !supportedImageExtensions[ext] {
		return fmt.Errorf("unsupported file type: %s (supported: jpg, jpeg, png, gif, bmp, webp, heic, heif, tiff, tif)", ext)
	}

	return nil
}

// ExtractMetadata extracts metadata from image file
func ExtractMetadata(filePath string) (*model.FileMetadata, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	metadata := &model.FileMetadata{
		Title:            filepath.Base(filePath),
		FileSize:         info.Size(),
		FileType:         model.FileTypeImage,
		LocalModified:    info.ModTime(),
		ModificationTime: info.ModTime().UnixMicro(),
	}

	// Try to extract creation time (Windows has birth time, Unix uses modification time)
	metadata.LocalCreated = info.ModTime()
	metadata.CreationTime = metadata.LocalCreated.UnixMicro()

	// Try to extract EXIF data
	file, err := os.Open(filePath)
	if err != nil {
		// If we can't open the file for EXIF, just return basic metadata
		return metadata, nil
	}
	defer file.Close()

	exifData, err := exif.Decode(file)
	if err != nil {
		// Not all images have EXIF data, this is not an error
		return metadata, nil
	}

	// Extract creation time from EXIF
	if dt, err := exifData.DateTime(); err == nil {
		metadata.CreationTime = dt.UnixMicro()
		metadata.LocalCreated = dt
	}

	// Extract GPS coordinates
	if lat, lon, err := exifData.LatLong(); err == nil {
		metadata.Latitude = lat
		metadata.Longitude = lon
	}

	// Extract image dimensions
	if tag, err := exifData.Get(exif.PixelXDimension); err == nil {
		if width, err := tag.Int(0); err == nil {
			metadata.Width = width
		}
	}
	if tag, err := exifData.Get(exif.PixelYDimension); err == nil {
		if height, err := tag.Int(0); err == nil {
			metadata.Height = height
		}
	}

	// Fallback to ExifImageWidth/ExifImageLength if PixelX/YDimension not found
	if metadata.Width == 0 {
		if tag, err := exifData.Get(exif.ImageWidth); err == nil {
			if width, err := tag.Int(0); err == nil {
				metadata.Width = width
			}
		}
	}
	if metadata.Height == 0 {
		if tag, err := exifData.Get(exif.ImageLength); err == nil {
			if height, err := tag.Int(0); err == nil {
				metadata.Height = height
			}
		}
	}

	return metadata, nil
}

// DetectFileType determines the file type (currently only supports images)
func DetectFileType(filePath string) (int, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if supportedImageExtensions[ext] {
		return model.FileTypeImage, nil
	}
	return -1, fmt.Errorf("unsupported file type: %s", ext)
}

// IsImageFile checks if a file is a supported image
func IsImageFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	return supportedImageExtensions[ext]
}
