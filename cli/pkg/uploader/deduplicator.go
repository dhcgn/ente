package uploader

import (
	"context"
	"fmt"
	"github.com/ente-io/cli/internal/api"
)

// CheckLocalDuplicate checks if a file with the given hash already exists locally
func CheckLocalDuplicate(ctx context.Context, storage Storage, hash string) (fileID int64, found bool, err error) {
	fileID, err = storage.GetFileIDByHash(ctx, hash)
	if err != nil {
		return 0, false, fmt.Errorf("failed to check local duplicate: %w", err)
	}
	if fileID == 0 {
		return 0, false, nil
	}
	return fileID, true, nil
}

// CheckRemoteDuplicate checks if a file with the given hash exists on the server
// This is an optional safety check - the server will also reject duplicates
func CheckRemoteDuplicate(ctx context.Context, client *api.Client, hash string, collectionID int64) (fileID int64, exists bool, err error) {
	// For now, we rely on local cache. Remote duplicate check would require:
	// 1. Fetching all files in the collection
	// 2. Comparing hashes
	// This is expensive and the server will reject duplicates anyway
	// So we skip remote check for performance
	return 0, false, nil
}

// StoreHashMapping saves the hash -> fileID mapping after successful upload
func StoreHashMapping(ctx context.Context, storage Storage, hash string, fileID int64) error {
	if err := storage.SaveFileHash(ctx, hash, fileID); err != nil {
		return fmt.Errorf("failed to store hash mapping: %w", err)
	}
	return nil
}
