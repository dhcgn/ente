package pkg

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/ente-io/cli/pkg/model"
	"github.com/ente-io/cli/pkg/uploader"
	bolt "go.etcd.io/bbolt"
)

// Upload uploads files to Ente Photos
func (c *ClICtrl) Upload(files []string, albumName string, config model.UploadConfig) (*uploader.UploadSummary, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files to upload")
	}

	// Get accounts
	accounts, err := c.GetAccounts(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get accounts: %w", err)
	}

	if len(accounts) == 0 {
		return nil, fmt.Errorf("no accounts found. Add an account using 'ente account add'")
	}

	// Find first Photos account
	var account model.Account
	found := false
	for _, acc := range accounts {
		if acc.App == "photos" {
			account = acc
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("no Photos account found. Add an account using 'ente account add'")
	}

	fmt.Printf("Using account: %s\n", account.Email)

	// Load secrets
	accSecretInfo, err := c.KeyHolder.LoadSecrets(account)
	if err != nil {
		return nil, fmt.Errorf("failed to load secrets: %w", err)
	}

	// Create context with account information (same as buildRequestContext in sync.go)
	ctx := context.Background()
	ctx = context.WithValue(ctx, "app", string(account.App))
	ctx = context.WithValue(ctx, "account_key", account.AccountKey())
	ctx = context.WithValue(ctx, "user_id", account.UserID)
	ctx = context.WithValue(ctx, "token", accSecretInfo.Token)

	// Set auth token for API client
	c.Client.AddToken(account.AccountKey(), base64.URLEncoding.EncodeToString(accSecretInfo.Token))

	// Create buckets if they don't exist
	if err := c.createUploadBuckets(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize upload buckets: %w", err)
	}

	// Create uploader
	uploader := uploader.NewUploader(ctx, c.Client, c, c.KeyHolder, config)

	// Upload files
	summary, err := uploader.UploadFiles(files, albumName)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}

	return summary, nil
}

// createUploadBuckets ensures upload-related buckets exist in BoltDB
func (c *ClICtrl) createUploadBuckets(ctx context.Context) error {
	accountKey := ctx.Value("account_key").(string)

	return c.DB.Update(func(tx *bolt.Tx) error {
		// Create account bucket if it doesn't exist
		accountBucket, err := tx.CreateBucketIfNotExists([]byte(accountKey))
		if err != nil {
			return fmt.Errorf("failed to create account bucket: %w", err)
		}

		// Create standard buckets (same as export)
		for _, subBucket := range []model.PhotosStore{model.KVConfig, model.RemoteAlbums, model.RemoteFiles, model.RemoteAlbumEntries} {
			if _, err := accountBucket.CreateBucketIfNotExists([]byte(subBucket)); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", subBucket, err)
			}
		}

		// Create UploadStates bucket
		if _, err := accountBucket.CreateBucketIfNotExists([]byte(model.UploadStates)); err != nil {
			return fmt.Errorf("failed to create UploadStates bucket: %w", err)
		}

		// Create FileHashes bucket
		if _, err := accountBucket.CreateBucketIfNotExists([]byte(model.FileHashes)); err != nil {
			return fmt.Errorf("failed to create FileHashes bucket: %w", err)
		}

		return nil
	})
}
