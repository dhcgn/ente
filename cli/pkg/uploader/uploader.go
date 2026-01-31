package uploader

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ente-io/cli/internal/api"
	"github.com/ente-io/cli/pkg/model"
	"github.com/ente-io/cli/pkg/secrets"
	"github.com/ente-io/cli/utils/encoding"
	"os"
	"path/filepath"
	"sync"
)

// Uploader handles file uploads to Ente
type Uploader struct {
	client        *api.Client
	storage       Storage
	keyHolder     *secrets.KeyHolder
	ctx           context.Context
	config        model.UploadConfig
	progressTrack *ProgressTracker
}

// NewUploader creates a new Uploader instance
func NewUploader(ctx context.Context, client *api.Client, storage Storage, keyHolder *secrets.KeyHolder, config model.UploadConfig) *Uploader {
	return &Uploader{
		client:    client,
		storage:   storage,
		keyHolder: keyHolder,
		ctx:       ctx,
		config:    config,
	}
}

// UploadFiles uploads multiple files to the specified album
func (u *Uploader) UploadFiles(files []string, albumName string) (*UploadSummary, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files to upload")
	}

	// Resolve or create album
	collectionID, collectionKey, err := GetOrCreateAlbum(u.ctx, u.client, u.keyHolder, albumName, u.config.CreateAlbum)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create album: %w", err)
	}

	fmt.Printf("Uploading to album '%s' (ID: %d)\n", albumName, collectionID)

	// Calculate total size
	var totalBytes int64
	for _, file := range files {
		if info, err := os.Stat(file); err == nil {
			totalBytes += info.Size()
		}
	}

	// Initialize progress tracker
	u.progressTrack = NewProgressTracker(len(files), totalBytes)

	// Create worker pool
	fileChan := make(chan string, len(files))
	resultChan := make(chan *UploadResult, len(files))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < u.config.Workers; i++ {
		wg.Add(1)
		go u.uploadWorker(collectionID, collectionKey, fileChan, resultChan, &wg)
	}

	// Queue files
	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)

	// Wait for workers to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results and show progress
	summary := &UploadSummary{
		TotalFiles: len(files),
		Errors:     make([]UploadError, 0),
	}

	for result := range resultChan {
		if result.Success {
			summary.CompletedFiles++
			summary.UploadedBytes += result.UploadedBytes
		} else if result.Skipped {
			summary.SkippedFiles++
		} else {
			summary.FailedFiles++
			summary.Errors = append(summary.Errors, UploadError{
				FileName: result.FileName,
				Error:    result.Error,
			})
		}

		// Print progress
		fmt.Printf("\r%s", u.progressTrack.Render())
	}

	fmt.Println() // New line after progress

	return summary, nil
}

// uploadWorker processes files from the file channel
func (u *Uploader) uploadWorker(collectionID int64, collectionKey []byte, fileChan <-chan string, resultChan chan<- *UploadResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for filePath := range fileChan {
		result := u.uploadFile(filePath, collectionID, collectionKey)
		resultChan <- result
	}
}

// uploadFile uploads a single file
func (u *Uploader) uploadFile(filePath string, collectionID int64, collectionKey []byte) *UploadResult {
	u.progressTrack.SetCurrentFile(filepath.Base(filePath))

	result := &UploadResult{
		FileName: filepath.Base(filePath),
	}

	// Step 1: Validate file
	if err := ValidateImageFile(filePath); err != nil {
		result.Error = fmt.Errorf("validation failed: %w", err)
		u.progressTrack.AddFailedFile()
		return result
	}

	// Step 2: Compute hash
	fileHash, err := ComputeFileHash(filePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to compute hash: %w", err)
		u.progressTrack.AddFailedFile()
		return result
	}

	// Step 3: Check deduplication
	if !u.config.ForceUpload {
		fileID, found, err := CheckLocalDuplicate(u.ctx, u.storage, fileHash)
		if err != nil {
			result.Error = fmt.Errorf("failed to check duplicate: %w", err)
			u.progressTrack.AddFailedFile()
			return result
		}
		if found {
			result.Skipped = true
			result.FileID = fileID
			u.progressTrack.AddSkippedFile()
			return result
		}
	}

	// Step 4: Extract metadata
	metadata, err := ExtractMetadata(filePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to extract metadata: %w", err)
		u.progressTrack.AddFailedFile()
		return result
	}

	// Step 5: Generate thumbnail
	thumbnailData, err := GenerateThumbnail(filePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to generate thumbnail: %w", err)
		u.progressTrack.AddFailedFile()
		return result
	}

	// Step 6: Encrypt everything
	fmt.Printf("\n=== Starting encryption for: %s ===\n", filepath.Base(filePath))
	encryptedData, err := u.encryptFileData(filePath, thumbnailData, metadata, collectionKey)
	if err != nil {
		result.Error = fmt.Errorf("failed to encrypt data: %w", err)
		u.progressTrack.AddFailedFile()
		return result
	}
	defer os.RemoveAll(encryptedData.TempDir)

	// Debug: Show encrypted data summary
	fmt.Printf("\nDebug [Upload] - Encrypted data summary:\n")
	fmt.Printf("  File decryption header: %s (len=%d)\n",
		encryptedData.FileDecryptionHeader, len(encryptedData.FileDecryptionHeader))
	fmt.Printf("  Encrypted file size: %d bytes\n", encryptedData.EncryptedFileSize)
	fmt.Printf("  Thumbnail decryption header: %s (len=%d)\n",
		encryptedData.ThumbnailDecryptionHeader, len(encryptedData.ThumbnailDecryptionHeader))
	fmt.Printf("  Encrypted thumbnail size: %d bytes\n", encryptedData.EncryptedThumbnailSize)

	// Step 7: Upload to S3
	if err := u.uploadToS3(encryptedData); err != nil {
		result.Error = fmt.Errorf("failed to upload to S3: %w", err)
		u.progressTrack.AddFailedFile()
		return result
	}

	// Step 8: Finalize via API
	fileID, err := u.finalizeUpload(collectionID, encryptedData.EncryptedUploadData)
	if err != nil {
		result.Error = fmt.Errorf("failed to finalize upload: %w", err)
		u.progressTrack.AddFailedFile()
		return result
	}

	// Step 9: Store hash mapping
	if err := StoreHashMapping(u.ctx, u.storage, fileHash, fileID); err != nil {
		// Non-fatal error, just log it
		fmt.Printf("\nWarning: failed to store hash mapping: %v\n", err)
	}

	result.Success = true
	result.FileID = fileID
	result.UploadedBytes = metadata.FileSize
	u.progressTrack.AddCompletedFile()
	u.progressTrack.AddUploadedBytes(metadata.FileSize)

	return result
}

// EncryptedFileData holds encrypted file data and paths
type EncryptedFileData struct {
	*model.EncryptedUploadData
	EncryptedFilePath      string
	EncryptedThumbnailData []byte
	TempDir                string
}

// encryptFileData encrypts file, thumbnail, and metadata
func (u *Uploader) encryptFileData(filePath string, thumbnailData []byte, metadata *model.FileMetadata, collectionKey []byte) (*EncryptedFileData, error) {
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

// uploadToS3 uploads encrypted file and thumbnail to S3
func (u *Uploader) uploadToS3(data *EncryptedFileData) error {
	// Upload file and get object key
	objectKey, err := u.uploadFileToS3(data.EncryptedFilePath, data.EncryptedFileSize)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}
	data.FileObjectKey = objectKey

	// Upload thumbnail and get object key
	thumbnailObjectKey, err := u.uploadThumbnailToS3(data.EncryptedThumbnailData)
	if err != nil {
		return fmt.Errorf("failed to upload thumbnail: %w", err)
	}
	data.ThumbnailObjectKey = thumbnailObjectKey

	return nil
}

// uploadFileToS3 uploads the encrypted file to S3 and returns the object key
func (u *Uploader) uploadFileToS3(encryptedFilePath string, fileSize int64) (string, error) {
	// Determine if we need multipart upload
	if fileSize >= u.config.MultipartMin {
		return u.uploadFileMultipart(encryptedFilePath, fileSize)
	}
	return u.uploadFileSingle(encryptedFilePath)
}

// uploadFileSingle uploads a file using single PUT request
func (u *Uploader) uploadFileSingle(encryptedFilePath string) (string, error) {
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
	uploadURL, err := u.client.GetUploadURL(u.ctx, fileInfo.Size(), md5Hash)
	if err != nil {
		return "", fmt.Errorf("failed to get upload URL: %w", err)
	}

	// Upload to S3
	if err := UploadToS3(uploadURL.URL, encryptedFilePath, md5Hash); err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	return uploadURL.ObjectKey, nil
}

// uploadFileMultipart uploads a large file using multipart upload
func (u *Uploader) uploadFileMultipart(encryptedFilePath string, fileSize int64) (string, error) {
	// Compute part MD5s
	partMD5s, err := ComputePartMD5s(encryptedFilePath, multipartPartSize)
	if err != nil {
		return "", fmt.Errorf("failed to compute part MD5s: %w", err)
	}

	// Request multipart upload URLs
	partCount := len(partMD5s)
	urls, err := u.client.GetMultipartUploadURLs(u.ctx, partCount, fileSize, multipartPartSize, partMD5s)
	if err != nil {
		return "", fmt.Errorf("failed to get multipart upload URLs: %w", err)
	}

	// Upload parts
	if err := UploadMultipart(urls, encryptedFilePath, partMD5s, u.progressTrack); err != nil {
		return "", fmt.Errorf("failed to upload multipart: %w", err)
	}

	return urls.ObjectKey, nil
}

// uploadThumbnailToS3 uploads the encrypted thumbnail to S3 and returns the object key
func (u *Uploader) uploadThumbnailToS3(thumbnailData []byte) (string, error) {
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
	uploadURL, err := u.client.GetUploadURL(u.ctx, int64(len(thumbnailData)), md5Hash)
	if err != nil {
		return "", fmt.Errorf("failed to get upload URL: %w", err)
	}

	// Upload to S3
	if err := UploadToS3(uploadURL.URL, tempFile.Name(), md5Hash); err != nil {
		return "", fmt.Errorf("failed to upload thumbnail to S3: %w", err)
	}

	return uploadURL.ObjectKey, nil
}

// finalizeUpload creates the file metadata on the server
func (u *Uploader) finalizeUpload(collectionID int64, data *model.EncryptedUploadData) (int64, error) {
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

	// Debug output - show full JSON
	reqJSON, _ := json.MarshalIndent(req, "", "  ")
	fmt.Printf("\nDebug - Finalize request JSON:\n%s\n", string(reqJSON))

	file, err := u.client.CreateFile(u.ctx, req)
	if err != nil {
		return 0, fmt.Errorf("failed to create file: %w", err)
	}

	return file.ID, nil
}
