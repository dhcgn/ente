package uploader

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ente-io/cli/internal/api"
	"github.com/ente-io/cli/pkg/model"
	"github.com/ente-io/cli/utils/encoding"
	"os"
	"path/filepath"
)

// UploadSingleFile uploads a single file to a specific collection
// This is a helper for the watch feature to upload files without the full uploader infrastructure
// Returns (fileID, uploadedBytes, error)
func UploadSingleFile(ctx context.Context, client *api.Client, storage Storage, filePath string, collectionID int64, collectionKey []byte) (int64, int64, error) {
	// Step 1: Validate file
	if err := ValidateImageFile(filePath); err != nil {
		return 0, 0, fmt.Errorf("validation failed: %w", err)
	}

	// Step 2: Compute hash
	fileHash, err := ComputeFileHash(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to compute hash: %w", err)
	}

	// Step 3: Check deduplication
	fileID, found, err := CheckLocalDuplicate(ctx, storage, fileHash)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to check duplicate: %w", err)
	}
	if found {
		return fileID, 0, fmt.Errorf("duplicate file found")
	}

	// Step 4: Extract metadata
	metadata, err := ExtractMetadata(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to extract metadata: %w", err)
	}

	// Step 5: Generate thumbnail
	thumbnailData, err := GenerateThumbnail(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to generate thumbnail: %w", err)
	}

	// Step 6: Encrypt everything
	encryptedData, err := encryptFileDataHelper(filePath, thumbnailData, metadata, collectionKey)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to encrypt data: %w", err)
	}
	defer os.RemoveAll(encryptedData.TempDir)

	// Step 7: Upload to S3
	if err := uploadToS3Helper(ctx, client, encryptedData); err != nil {
		return 0, 0, fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Step 8: Finalize via API
	fileID, err = finalizeUploadHelper(ctx, client, collectionID, encryptedData.EncryptedUploadData)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to finalize upload: %w", err)
	}

	// Step 9: Store hash mapping
	if err := StoreHashMapping(ctx, storage, fileHash, fileID); err != nil {
		// Non-fatal error, just log it
		fmt.Printf("\nWarning: failed to store hash mapping: %v\n", err)
	}

	return fileID, metadata.FileSize, nil
}

// encryptFileDataHelper encrypts file, thumbnail, and metadata
func encryptFileDataHelper(filePath string, thumbnailData []byte, metadata *model.FileMetadata, collectionKey []byte) (*EncryptedFileData, error) {
	// Generate file key
	fileKey, err := GenerateFileKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate file key: %w", err)
	}

	// Encrypt file key with collection key
	encryptedFileKey, keyNonce, err := EncryptFileKeyWithCollectionKey(fileKey, collectionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt file key: %w", err)
	}

	// Create temp directory for encrypted files
	tempDir, err := os.MkdirTemp("", "ente-upload-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Encrypt file
	encryptedFilePath := filepath.Join(tempDir, "encrypted_file")
	fileNonce, encryptedFileSize, err := EncryptFile(filePath, encryptedFilePath, fileKey)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to encrypt file: %w", err)
	}

	// Encrypt thumbnail
	encryptedThumbnail, thumbnailNonce, err := EncryptData(thumbnailData, fileKey)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to encrypt thumbnail: %w", err)
	}

	// Encrypt metadata
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	encryptedMetadata, metadataNonce, err := EncryptData(metadataJSON, fileKey)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to encrypt metadata: %w", err)
	}

	return &EncryptedFileData{
		EncryptedUploadData: &model.EncryptedUploadData{
			FileKey:                   fileKey,
			EncryptedFileKey:          encryptedFileKey,
			KeyDecryptionNonce:        keyNonce,
			FileObjectKey:             "", // Set after S3 upload
			FileDecryptionHeader:      encoding.EncodeBase64(fileNonce),
			EncryptedFileSize:         encryptedFileSize,
			ThumbnailObjectKey:        "", // Set after S3 upload
			ThumbnailDecryptionHeader: encoding.EncodeBase64(thumbnailNonce),
			EncryptedThumbnailSize:    int64(len(encryptedThumbnail)),
			EncryptedMetadata:         encoding.EncodeBase64(encryptedMetadata),
			MetadataDecryptionHeader:  encoding.EncodeBase64(metadataNonce),
		},
		EncryptedFilePath:      encryptedFilePath,
		EncryptedThumbnailData: encryptedThumbnail,
		TempDir:                tempDir,
	}, nil
}

// uploadToS3Helper uploads encrypted file and thumbnail to S3
func uploadToS3Helper(ctx context.Context, client *api.Client, data *EncryptedFileData) error {
	// Upload file
	objectKey, err := uploadFileToS3Helper(ctx, client, data.EncryptedFilePath, data.EncryptedFileSize)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}
	data.FileObjectKey = objectKey

	// Upload thumbnail
	thumbnailObjectKey, err := uploadThumbnailToS3Helper(ctx, client, data.EncryptedThumbnailData)
	if err != nil {
		return fmt.Errorf("failed to upload thumbnail: %w", err)
	}
	data.ThumbnailObjectKey = thumbnailObjectKey

	return nil
}

// uploadFileToS3Helper uploads the encrypted file to S3 and returns the object key
func uploadFileToS3Helper(ctx context.Context, client *api.Client, encryptedFilePath string, fileSize int64) (string, error) {
	// Determine if we need multipart upload
	if fileSize >= DefaultMultipartMin {
		return uploadFileMultipartHelper(ctx, client, encryptedFilePath, fileSize)
	}
	return uploadFileSingleHelper(ctx, client, encryptedFilePath)
}

// uploadFileSingleHelper uploads a file using single PUT request
func uploadFileSingleHelper(ctx context.Context, client *api.Client, encryptedFilePath string) (string, error) {
	// Compute MD5
	md5Hash, err := ComputeFileMD5(encryptedFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to compute MD5: %w", err)
	}

	// Get file size
	fileInfo, err := os.Stat(encryptedFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	// Request upload URL
	uploadURL, err := client.GetUploadURL(ctx, fileInfo.Size(), md5Hash)
	if err != nil {
		return "", fmt.Errorf("failed to get upload URL: %w", err)
	}

	// Upload to S3
	if err := UploadToS3(uploadURL.URL, encryptedFilePath, md5Hash); err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	return uploadURL.ObjectKey, nil
}

// uploadFileMultipartHelper uploads a large file using multipart upload
func uploadFileMultipartHelper(ctx context.Context, client *api.Client, encryptedFilePath string, fileSize int64) (string, error) {
	// Compute part MD5s
	partSize := int64(multipartPartSize)
	partMD5s, err := ComputePartMD5s(encryptedFilePath, multipartPartSize)
	if err != nil {
		return "", fmt.Errorf("failed to compute part MD5s: %w", err)
	}

	// Request multipart upload URLs
	urls, err := client.GetMultipartUploadURLs(ctx, fileSize, partSize, partMD5s)
	if err != nil {
		return "", fmt.Errorf("failed to get multipart upload URLs: %w", err)
	}

	// Upload parts
	if err := UploadMultipart(urls, encryptedFilePath, partMD5s, nil); err != nil {
		return "", fmt.Errorf("failed to upload multipart: %w", err)
	}

	return urls.ObjectKey, nil
}

// uploadThumbnailToS3Helper uploads the encrypted thumbnail to S3 and returns the object key
func uploadThumbnailToS3Helper(ctx context.Context, client *api.Client, thumbnailData []byte) (string, error) {
	// Write thumbnail to temp file
	tempFile, err := os.CreateTemp("", "ente-thumbnail-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	if _, err := tempFile.Write(thumbnailData); err != nil {
		return "", fmt.Errorf("failed to write thumbnail: %w", err)
	}
	tempFile.Close()

	// Compute MD5
	md5Hash, err := ComputeFileMD5(tempFile.Name())
	if err != nil {
		return "", fmt.Errorf("failed to compute MD5: %w", err)
	}

	// Request upload URL
	uploadURL, err := client.GetUploadURL(ctx, int64(len(thumbnailData)), md5Hash)
	if err != nil {
		return "", fmt.Errorf("failed to get upload URL: %w", err)
	}

	// Upload to S3
	if err := UploadToS3(uploadURL.URL, tempFile.Name(), md5Hash); err != nil {
		return "", fmt.Errorf("failed to upload thumbnail to S3: %w", err)
	}

	return uploadURL.ObjectKey, nil
}

// finalizeUploadHelper creates the file metadata on the server
func finalizeUploadHelper(ctx context.Context, client *api.Client, collectionID int64, data *model.EncryptedUploadData) (int64, error) {
	req := api.FileCreateRequest{
		CollectionID:       collectionID,
		EncryptedKey:       data.EncryptedFileKey,
		KeyDecryptionNonce: data.KeyDecryptionNonce,
		File: api.UploadFileAttributes{
			ObjectKey:        data.FileObjectKey,
			DecryptionHeader: data.FileDecryptionHeader,
			Size:             data.EncryptedFileSize,
		},
		Thumbnail: api.UploadFileAttributes{
			ObjectKey:        data.ThumbnailObjectKey,
			DecryptionHeader: data.ThumbnailDecryptionHeader,
			Size:             data.EncryptedThumbnailSize,
		},
		Metadata: api.UploadMetadataAttributes{
			EncryptedData:    data.EncryptedMetadata,
			DecryptionHeader: data.MetadataDecryptionHeader,
		},
	}

	file, err := client.CreateFile(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("failed to create file: %w", err)
	}

	return file.ID, nil
}
