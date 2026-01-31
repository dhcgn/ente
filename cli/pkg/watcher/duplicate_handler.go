package watcher

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/ente-io/cli/internal/api"
	"github.com/ente-io/cli/internal/crypto"
	"github.com/ente-io/cli/pkg/model"
	"github.com/ente-io/cli/pkg/secrets"
	"github.com/ente-io/cli/utils/encoding"
	"golang.org/x/crypto/nacl/secretbox"
	"time"
)

// DuplicateHandler handles checking and adding duplicate files to albums
type DuplicateHandler struct {
	ctx       context.Context
	client    *api.Client
	storage   Storage
	keyHolder *secrets.KeyHolder
}

// Storage interface for accessing BoltDB
type Storage interface {
	GetFileHashMapping(ctx context.Context, fileHash string) (*model.FileHashMapping, error)
	GetFileIDByHash(ctx context.Context, fileHash string) (int64, error)
	SaveFileHashMapping(ctx context.Context, fileHash string, fileID int64, collectionID int64) error
	SaveFileHash(ctx context.Context, fileHash string, fileID int64) error
	SaveProcessedFile(ctx context.Context, file *model.ProcessedFile) error
	SaveWatchState(ctx context.Context, state *model.WatchState) error
}

// NewDuplicateHandler creates a new DuplicateHandler
func NewDuplicateHandler(ctx context.Context, client *api.Client, storage Storage, keyHolder *secrets.KeyHolder) *DuplicateHandler {
	return &DuplicateHandler{
		ctx:       ctx,
		client:    client,
		storage:   storage,
		keyHolder: keyHolder,
	}
}

// CheckAndHandleDuplicate checks if a file is a duplicate and handles adding it to the target album
// Returns (fileID, collectionID, isDuplicate, error)
func (dh *DuplicateHandler) CheckAndHandleDuplicate(fileHash string, filePath string, targetCollectionID int64, targetCollectionKey []byte) (int64, int64, bool, error) {
	// Check local duplicate
	mapping, err := dh.storage.GetFileHashMapping(dh.ctx, fileHash)
	if err != nil {
		return 0, 0, false, fmt.Errorf("failed to check duplicate: %w", err)
	}

	if mapping == nil {
		// Not a duplicate - file needs to be uploaded
		return 0, 0, false, nil
	}

	// File exists - check if it's in the target album
	if mapping.CollectionID == targetCollectionID {
		// Already in target album
		processedFile := &model.ProcessedFile{
			FilePath:     filePath,
			FileHash:     fileHash,
			FileID:       mapping.FileID,
			CollectionID: targetCollectionID,
			ProcessedAt:  time.Now().Unix(),
			Status:       model.StatusDuplicate,
		}
		if err := dh.storage.SaveProcessedFile(dh.ctx, processedFile); err != nil {
			fmt.Printf("Warning: failed to save processed file: %v\n", err)
		}
		return mapping.FileID, targetCollectionID, true, nil
	}

	// File exists in different album - add it to target album
	if err := dh.addFileToAlbum(mapping.FileID, mapping.CollectionID, targetCollectionID, targetCollectionKey); err != nil {
		return 0, 0, false, fmt.Errorf("failed to add file to album: %w", err)
	}

	// Save processed file record
	processedFile := &model.ProcessedFile{
		FilePath:     filePath,
		FileHash:     fileHash,
		FileID:       mapping.FileID,
		CollectionID: targetCollectionID,
		ProcessedAt:  time.Now().Unix(),
		Status:       model.StatusDuplicate,
	}
	if err := dh.storage.SaveProcessedFile(dh.ctx, processedFile); err != nil {
		fmt.Printf("Warning: failed to save processed file: %v\n", err)
	}

	return mapping.FileID, targetCollectionID, true, nil
}

// addFileToAlbum adds an existing file to a target album by re-encrypting its key
func (dh *DuplicateHandler) addFileToAlbum(fileID int64, originalCollectionID int64, targetCollectionID int64, targetCollectionKey []byte) error {
	// Fetch file metadata from server
	file, err := dh.client.GetFile(dh.ctx, originalCollectionID, fileID)
	if err != nil {
		return fmt.Errorf("failed to fetch file metadata: %w", err)
	}

	// Get original collection key
	originalCollectionKey, err := dh.getCollectionKey(originalCollectionID)
	if err != nil {
		return fmt.Errorf("failed to get original collection key: %w", err)
	}

	// Decrypt file key using original collection key
	fileKey, err := dh.decryptFileKey(file.EncryptedKey, file.KeyDecryptionNonce, originalCollectionKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt file key: %w", err)
	}

	// Re-encrypt file key with target collection key
	encryptedKey, keyNonce, err := dh.encryptFileKey(fileKey, targetCollectionKey)
	if err != nil {
		return fmt.Errorf("failed to re-encrypt file key: %w", err)
	}

	// Call API to add file to collection
	fileItem := api.CollectionFileItem{
		ID:                 fileID,
		EncryptedKey:       encryptedKey,
		KeyDecryptionNonce: keyNonce,
	}

	if err := dh.client.AddFilesToCollection(dh.ctx, targetCollectionID, []api.CollectionFileItem{fileItem}); err != nil {
		return fmt.Errorf("failed to add file to collection via API: %w", err)
	}

	return nil
}

// getCollectionKey retrieves the collection key for a given collection ID
func (dh *DuplicateHandler) getCollectionKey(collectionID int64) ([]byte, error) {
	// Fetch all collections
	collections, err := dh.client.GetAllCollections(dh.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch collections: %w", err)
	}

	// Find the target collection
	for _, collection := range collections {
		if collection.ID == collectionID {
			collectionKey, err := dh.keyHolder.GetCollectionKey(dh.ctx, collection)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt collection key: %w", err)
			}
			return collectionKey, nil
		}
	}

	return nil, fmt.Errorf("collection %d not found", collectionID)
}

// decryptFileKey decrypts the file key using the collection key
func (dh *DuplicateHandler) decryptFileKey(encryptedKey string, nonce string, collectionKey []byte) ([]byte, error) {
	encryptedBytes := encoding.DecodeBase64(encryptedKey)
	nonceBytes := encoding.DecodeBase64(nonce)

	fileKey, err := crypto.SecretBoxOpen(encryptedBytes, nonceBytes, collectionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt file key: %w", err)
	}

	return fileKey, nil
}

// encryptFileKey encrypts the file key using the collection key
func (dh *DuplicateHandler) encryptFileKey(fileKey []byte, collectionKey []byte) (encrypted string, nonce string, err error) {
	// Generate random nonce (24 bytes)
	nonceBytes := make([]byte, 24)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Convert to fixed-size arrays
	var keyArray [32]byte
	var nonceArray [24]byte
	copy(keyArray[:], collectionKey)
	copy(nonceArray[:], nonceBytes)

	// Encrypt using SecretBox
	encryptedBytes := secretbox.Seal(nil, fileKey, &nonceArray, &keyArray)

	return encoding.EncodeBase64(encryptedBytes), encoding.EncodeBase64(nonceBytes), nil
}

// SaveFileHashMapping saves the hash mapping after successful upload
func (dh *DuplicateHandler) SaveFileHashMapping(fileHash string, filePath string, fileID int64, collectionID int64) error {
	// Save hash mapping
	if err := dh.storage.SaveFileHashMapping(dh.ctx, fileHash, fileID, collectionID); err != nil {
		return fmt.Errorf("failed to save hash mapping: %w", err)
	}

	// Save processed file record
	processedFile := &model.ProcessedFile{
		FilePath:     filePath,
		FileHash:     fileHash,
		FileID:       fileID,
		CollectionID: collectionID,
		ProcessedAt:  time.Now().Unix(),
		Status:       model.StatusUploaded,
	}
	if err := dh.storage.SaveProcessedFile(dh.ctx, processedFile); err != nil {
		fmt.Printf("Warning: failed to save processed file: %v\n", err)
	}

	return nil
}

// FormatMessage returns a user-friendly message about the duplicate handling
func FormatMessage(fileName string, isDuplicate bool, addedToAlbum bool) string {
	if !isDuplicate {
		return fmt.Sprintf("✓ Uploaded: %s", fileName)
	}
	if addedToAlbum {
		return fmt.Sprintf("✓ Added to album: %s (duplicate)", fileName)
	}
	return fmt.Sprintf("○ Skipped: %s (already in album)", fileName)
}
