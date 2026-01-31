package uploader

import (
	"github.com/ente-io/cli/pkg/model"
)

// UploadResult represents the result of an upload operation
type UploadResult struct {
	FileName      string
	Success       bool
	Skipped       bool // True if file was skipped due to deduplication
	Error         error
	FileID        int64
	UploadedBytes int64
}

// UploadSummary provides overall statistics for an upload session
type UploadSummary struct {
	TotalFiles     int
	CompletedFiles int
	FailedFiles    int
	SkippedFiles   int
	TotalBytes     int64
	UploadedBytes  int64
	Errors         []UploadError
}

// UploadError captures details about a failed upload
type UploadError struct {
	FileName string
	Error    error
}

// Default configuration values
const (
	DefaultWorkers      = 4
	DefaultMultipartMin = 20 * 1024 * 1024 // 20MB
	DefaultChunkSize    = 4 * 1024 * 1024  // 4MB
	DefaultAlbumName    = "CLI Uploads"
)

// NewUploadConfig creates a default upload configuration
func NewUploadConfig() model.UploadConfig {
	return model.UploadConfig{
		Workers:      DefaultWorkers,
		ForceUpload:  false,
		CreateAlbum:  false,
		ChunkSize:    DefaultChunkSize,
		MultipartMin: DefaultMultipartMin,
	}
}
