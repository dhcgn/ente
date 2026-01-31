package api

import (
	"context"
	"fmt"
)

// UploadURL represents a single upload URL response
type UploadURL struct {
	URL       string `json:"url"`
	ObjectKey string `json:"objectKey"`
}

// MultipartUploadURLs represents multipart upload URLs response
type MultipartUploadURLs struct {
	ObjectKey   string   `json:"objectKey"`
	PartURLs    []string `json:"partURLs"`
	CompleteURL string   `json:"completeURL"`
}

// GetUploadURLRequest contains parameters for requesting an upload URL
type GetUploadURLRequest struct {
	ContentLength int64  `json:"contentLength"`
	ContentMD5    string `json:"contentMD5"` // base64-encoded MD5
}

// GetMultipartUploadURLsRequest contains parameters for multipart upload
type GetMultipartUploadURLsRequest struct {
	ContentLength int64    `json:"contentLength"` // Total file size
	PartLength    int64    `json:"partLength"`    // Size of each part
	PartMD5s      []string `json:"partMd5s"`      // MD5 for each part (base64-encoded)
}

// FileCreateRequest represents the file creation payload
type FileCreateRequest struct {
	CollectionID       int64                    `json:"collectionID"`
	EncryptedKey       string                   `json:"encryptedKey"`
	KeyDecryptionNonce string                   `json:"keyDecryptionNonce"`
	File               UploadFileAttributes     `json:"file"`
	Thumbnail          UploadFileAttributes     `json:"thumbnail"`
	Metadata           UploadMetadataAttributes `json:"metadata"`
	PubMagicMetadata   *MagicMetadata           `json:"pubMagicMetadata,omitempty"`
}

// UploadFileAttributes represents file/thumbnail data for upload
type UploadFileAttributes struct {
	ObjectKey        string `json:"objectKey"`
	DecryptionHeader string `json:"decryptionHeader"`
	Size             int64  `json:"size"`
}

// UploadMetadataAttributes represents metadata for upload
type UploadMetadataAttributes struct {
	EncryptedData    string `json:"encryptedData"`
	DecryptionHeader string `json:"decryptionHeader"`
}

// GetUploadURL requests a single upload URL from the server
func (c *Client) GetUploadURL(ctx context.Context, contentLength int64, contentMD5 string) (*UploadURL, error) {
	req := GetUploadURLRequest{
		ContentLength: contentLength,
		ContentMD5:    contentMD5,
	}

	var result UploadURL
	r, err := c.restClient.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&result).
		Post("/files/upload-url")

	if err != nil {
		return nil, fmt.Errorf("failed to get upload URL: %w", err)
	}

	if r.IsError() {
		return nil, &ApiError{
			StatusCode: r.StatusCode(),
			Message:    r.String(),
		}
	}

	return &result, nil
}

// GetMultipartUploadURLs requests multipart upload URLs for large files
func (c *Client) GetMultipartUploadURLs(ctx context.Context, fileSize, partSize int64, partMD5s []string) (*MultipartUploadURLs, error) {
	req := GetMultipartUploadURLsRequest{
		ContentLength: fileSize,
		PartLength:    partSize,
		PartMD5s:      partMD5s,
	}

	var result MultipartUploadURLs
	r, err := c.restClient.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&result).
		Post("/files/multipart-upload-url")

	if err != nil {
		return nil, fmt.Errorf("failed to get multipart upload URLs: %w", err)
	}

	if r.IsError() {
		return nil, &ApiError{
			StatusCode: r.StatusCode(),
			Message:    r.String(),
		}
	}

	return &result, nil
}

// CreateFile finalizes the upload by creating file metadata on the server
func (c *Client) CreateFile(ctx context.Context, req FileCreateRequest) (*File, error) {
	var result File
	r, err := c.restClient.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&result).
		Post("/files")

	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	if r.IsError() {
		return nil, &ApiError{
			StatusCode: r.StatusCode(),
			Message:    r.String(),
		}
	}

	return &result, nil
}

// CreateCollection creates a new collection (album)
func (c *Client) CreateCollection(ctx context.Context, req CreateCollectionRequest) (*Collection, error) {
	var res struct {
		Collection Collection `json:"collection"`
	}
	r, err := c.restClient.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&res).
		Post("/collections")

	if err != nil {
		return nil, fmt.Errorf("failed to create collection: %w", err)
	}

	if r.IsError() {
		return nil, &ApiError{
			StatusCode: r.StatusCode(),
			Message:    r.String(),
		}
	}

	return &res.Collection, nil
}

// CreateCollectionRequest represents the collection creation payload
type CreateCollectionRequest struct {
	EncryptedKey       string              `json:"encryptedKey"`
	KeyDecryptionNonce string              `json:"keyDecryptionNonce"`
	EncryptedName      string              `json:"encryptedName"`
	NameDecryptionNonce string             `json:"nameDecryptionNonce"`
	Type               CollectionType      `json:"type"`
	Attributes         CollectionAttributes `json:"attributes"`
}

// CollectionType represents the type of collection
type CollectionType string

const (
	CollectionTypeAlbum  CollectionType = "album"
	CollectionTypeFolder CollectionType = "folder"
)

// CollectionAttributes contains collection attributes
type CollectionAttributes struct {
	Version int `json:"version"`
}
