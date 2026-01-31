package pkg

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ente-io/cli/pkg/model"
	"log"
	"strconv"
	"time"

	bolt "go.etcd.io/bbolt"
)

func GetDB(path string) (*bolt.DB, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to open db %s ", path), err)
	}
	return db, err
}

func (c *ClICtrl) GetInt64ConfigValue(ctx context.Context, key string) (int64, error) {
	value, err := c.getConfigValue(ctx, key)
	if err != nil {
		return 0, err
	}
	var result int64
	if value != nil {
		result, err = strconv.ParseInt(string(value), 10, 64)
		if err != nil {
			return 0, err
		}
	}
	return result, nil
}

func (c *ClICtrl) getConfigValue(ctx context.Context, key string) ([]byte, error) {
	var value []byte
	err := c.DB.View(func(tx *bolt.Tx) error {
		kvBucket, err := getAccountStore(ctx, tx, model.KVConfig)
		if err != nil {
			return err
		}
		value = kvBucket.Get([]byte(key))
		return nil
	})
	return value, err
}

func (c *ClICtrl) GetAllValues(ctx context.Context, store model.PhotosStore) ([][]byte, error) {
	result := make([][]byte, 0)
	err := c.DB.View(func(tx *bolt.Tx) error {
		kvBucket, err := getAccountStore(ctx, tx, store)
		if err != nil {
			return err
		}
		kvBucket.ForEach(func(k, v []byte) error {
			result = append(result, v)
			return nil
		})
		return nil
	})
	return result, err
}

func (c *ClICtrl) PutConfigValue(ctx context.Context, key string, value []byte) error {
	return c.DB.Update(func(tx *bolt.Tx) error {
		kvBucket, err := getAccountStore(ctx, tx, model.KVConfig)
		if err != nil {
			return err
		}
		return kvBucket.Put([]byte(key), value)
	})
}
func (c *ClICtrl) PutValue(ctx context.Context, store model.PhotosStore, key []byte, value []byte) error {
	return c.DB.Update(func(tx *bolt.Tx) error {
		kvBucket, err := getAccountStore(ctx, tx, store)
		if err != nil {
			return err
		}
		return kvBucket.Put(key, value)
	})
}

func (c *ClICtrl) DeleteValue(ctx context.Context, store model.PhotosStore, key []byte) error {
	return c.DB.Update(func(tx *bolt.Tx) error {
		kvBucket, err := getAccountStore(ctx, tx, store)
		if err != nil {
			return err
		}
		return kvBucket.Delete(key)
	})
}

// GetValue
func (c *ClICtrl) GetValue(ctx context.Context, store model.PhotosStore, key []byte) ([]byte, error) {
	var value []byte
	err := c.DB.View(func(tx *bolt.Tx) error {
		kvBucket, err := getAccountStore(ctx, tx, store)
		if err != nil {
			return err
		}
		value = kvBucket.Get(key)
		return nil
	})
	return value, err
}
func getAccountStore(ctx context.Context, tx *bolt.Tx, storeType model.PhotosStore) (*bolt.Bucket, error) {
	accountKey := ctx.Value("account_key").(string)
	accountBucket := tx.Bucket([]byte(accountKey))
	if accountBucket == nil {
		return nil, fmt.Errorf("account bucket not found")
	}
	store := accountBucket.Bucket([]byte(storeType))
	if store == nil {
		return nil, fmt.Errorf("store %s not found", storeType)
	}
	return store, nil
}

// GetUploadState retrieves the upload state for a file by its hash
func (c *ClICtrl) GetUploadState(ctx context.Context, fileHash string) (*model.UploadState, error) {
	value, err := c.GetValue(ctx, model.UploadStates, []byte(fileHash))
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}

	var state model.UploadState
	if err := json.Unmarshal(value, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal upload state: %w", err)
	}
	return &state, nil
}

// SaveUploadState saves the upload state for a file
func (c *ClICtrl) SaveUploadState(ctx context.Context, state *model.UploadState) error {
	state.UpdatedAt = time.Now().UnixMicro()
	value, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal upload state: %w", err)
	}
	return c.PutValue(ctx, model.UploadStates, []byte(state.FileHash), value)
}

// GetFileIDByHash retrieves the file ID for a given hash (for deduplication)
// Deprecated: Use GetFileHashMapping instead for watch feature compatibility
func (c *ClICtrl) GetFileIDByHash(ctx context.Context, fileHash string) (int64, error) {
	mapping, err := c.GetFileHashMapping(ctx, fileHash)
	if err != nil {
		return 0, err
	}
	if mapping == nil {
		return 0, nil
	}
	return mapping.FileID, nil
}

// GetFileHashMapping retrieves the full file hash mapping (fileID and collectionID)
func (c *ClICtrl) GetFileHashMapping(ctx context.Context, fileHash string) (*model.FileHashMapping, error) {
	value, err := c.GetValue(ctx, model.FileHashes, []byte(fileHash))
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}

	var mapping model.FileHashMapping
	if err := json.Unmarshal(value, &mapping); err != nil {
		// Try legacy format (plain file ID as string)
		fileID, parseErr := strconv.ParseInt(string(value), 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse file hash mapping: %w", err)
		}
		return &model.FileHashMapping{
			FileID:       fileID,
			CollectionID: 0, // Legacy entries don't have collection ID
		}, nil
	}
	return &mapping, nil
}

// SaveFileHash saves the mapping from file hash to file ID
// Deprecated: Use SaveFileHashMapping instead for watch feature compatibility
func (c *ClICtrl) SaveFileHash(ctx context.Context, fileHash string, fileID int64) error {
	return c.SaveFileHashMapping(ctx, fileHash, fileID, 0)
}

// SaveFileHashMapping saves the complete mapping from file hash to file ID and collection ID
func (c *ClICtrl) SaveFileHashMapping(ctx context.Context, fileHash string, fileID int64, collectionID int64) error {
	mapping := model.FileHashMapping{
		FileID:       fileID,
		CollectionID: collectionID,
	}
	value, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("failed to marshal file hash mapping: %w", err)
	}
	return c.PutValue(ctx, model.FileHashes, []byte(fileHash), value)
}

// GetWatchState retrieves the watch state for a watch path
func (c *ClICtrl) GetWatchState(ctx context.Context, watchPath string) (*model.WatchState, error) {
	// Use hash of path as key to handle long paths
	key := fmt.Sprintf("%x", watchPath)
	value, err := c.GetValue(ctx, model.WatchStates, []byte(key))
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}

	var state model.WatchState
	if err := json.Unmarshal(value, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal watch state: %w", err)
	}
	return &state, nil
}

// SaveWatchState saves the watch state for a watch path
func (c *ClICtrl) SaveWatchState(ctx context.Context, state *model.WatchState) error {
	value, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal watch state: %w", err)
	}
	key := fmt.Sprintf("%x", state.WatchPath)
	return c.PutValue(ctx, model.WatchStates, []byte(key), value)
}

// GetProcessedFile retrieves the processed file record by file path
func (c *ClICtrl) GetProcessedFile(ctx context.Context, filePath string) (*model.ProcessedFile, error) {
	// Use hash of path as key
	key := fmt.Sprintf("%x", filePath)
	value, err := c.GetValue(ctx, model.WatchFiles, []byte(key))
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}

	var file model.ProcessedFile
	if err := json.Unmarshal(value, &file); err != nil {
		return nil, fmt.Errorf("failed to unmarshal processed file: %w", err)
	}
	return &file, nil
}

// SaveProcessedFile saves the processed file record
func (c *ClICtrl) SaveProcessedFile(ctx context.Context, file *model.ProcessedFile) error {
	value, err := json.Marshal(file)
	if err != nil {
		return fmt.Errorf("failed to marshal processed file: %w", err)
	}
	key := fmt.Sprintf("%x", file.FilePath)
	return c.PutValue(ctx, model.WatchFiles, []byte(key), value)
}
