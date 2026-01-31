package uploader

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/ente-io/cli/internal/api"
	"github.com/ente-io/cli/internal/crypto"
	"github.com/ente-io/cli/pkg/secrets"
	"github.com/ente-io/cli/utils/encoding"
	"golang.org/x/crypto/nacl/secretbox"
)

// GetOrCreateAlbum finds an album by name or creates it if it doesn't exist
func GetOrCreateAlbum(ctx context.Context, client *api.Client, keyHolder *secrets.KeyHolder, albumName string, createIfMissing bool) (int64, []byte, error) {
	// Fetch all collections
	collections, err := client.GetAllCollections(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to fetch collections: %w", err)
	}

	// Decrypt names and search for match
	for _, collection := range collections {
		if collection.IsDeleted {
			continue
		}

		// Decrypt collection key
		collectionKey, err := keyHolder.GetCollectionKey(ctx, collection)
		if err != nil {
			// Skip collections we can't decrypt
			continue
		}

		// Decrypt collection name
		decryptedName, err := decryptCollectionName(collection, collectionKey)
		if err != nil {
			// Skip if we can't decrypt the name
			continue
		}

		// Check if name matches
		if decryptedName == albumName {
			return collection.ID, collectionKey, nil
		}
	}

	// Album not found
	if !createIfMissing {
		return 0, nil, fmt.Errorf("album '%s' not found", albumName)
	}

	// Create new album
	return createAlbum(ctx, client, keyHolder, albumName)
}

// decryptCollectionName decrypts the collection name
func decryptCollectionName(collection api.Collection, collectionKey []byte) (string, error) {
	encryptedName := encoding.DecodeBase64(collection.EncryptedName)
	nonce := encoding.DecodeBase64(collection.NameDecryptionNonce)

	decryptedBytes, err := crypto.SecretBoxOpen(encryptedName, nonce, collectionKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt collection name: %w", err)
	}

	return string(decryptedBytes), nil
}

// createAlbum creates a new album (collection)
func createAlbum(ctx context.Context, client *api.Client, keyHolder *secrets.KeyHolder, albumName string) (int64, []byte, error) {
	accSecretInfo := keyHolder.GetAccountSecretInfo(ctx)

	// Generate random collection key
	collectionKey := make([]byte, 32)
	if _, err := rand.Read(collectionKey); err != nil {
		return 0, nil, fmt.Errorf("failed to generate collection key: %w", err)
	}

	// Encrypt collection key with master key (SecretBox)
	encryptedKey, keyNonce, err := encryptWithSecretBox(collectionKey, accSecretInfo.MasterKey)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to encrypt collection key: %w", err)
	}

	// Encrypt album name with collection key
	encryptedName, nameNonce, err := encryptWithSecretBox([]byte(albumName), collectionKey)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to encrypt album name: %w", err)
	}

	// Create collection via API
	req := api.CreateCollectionRequest{
		EncryptedKey:        encryptedKey,
		KeyDecryptionNonce:  keyNonce,
		EncryptedName:       encryptedName,
		NameDecryptionNonce: nameNonce,
		Type:                api.CollectionTypeAlbum,
		Attributes: api.CollectionAttributes{
			Version: 1,
		},
	}

	collection, err := client.CreateCollection(ctx, req)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create collection: %w", err)
	}

	fmt.Printf("Debug - Created collection: ID=%d, Name=%s\n", collection.ID, albumName)
	return collection.ID, collectionKey, nil
}

// encryptWithSecretBox encrypts data using SecretBox and returns base64-encoded strings
func encryptWithSecretBox(data, key []byte) (encrypted, nonce string, err error) {
	// Generate random nonce
	nonceBytes := make([]byte, 24)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Convert to fixed-size arrays
	var keyArray [32]byte
	var nonceArray [24]byte
	copy(keyArray[:], key)
	copy(nonceArray[:], nonceBytes)

	// Encrypt using SecretBox
	encryptedBytes := secretbox.Seal(nil, data, &nonceArray, &keyArray)

	return encoding.EncodeBase64(encryptedBytes), encoding.EncodeBase64(nonceBytes), nil
}
