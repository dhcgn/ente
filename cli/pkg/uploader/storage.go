package uploader

import (
	"context"
)

// Storage defines the interface for upload state persistence
type Storage interface {
	GetFileIDByHash(ctx context.Context, fileHash string) (int64, error)
	SaveFileHash(ctx context.Context, fileHash string, fileID int64) error
}
