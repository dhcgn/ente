package model

import "time"

// UploadStatus represents the current state of an upload
type UploadStatus int

const (
	UploadStatusPending UploadStatus = iota
	UploadStatusEncrypting
	UploadStatusUploading
	UploadStatusCompleted
	UploadStatusFailed
)

func (s UploadStatus) String() string {
	switch s {
	case UploadStatusPending:
		return "pending"
	case UploadStatusEncrypting:
		return "encrypting"
	case UploadStatusUploading:
		return "uploading"
	case UploadStatusCompleted:
		return "completed"
	case UploadStatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// UploadState tracks the upload progress of a file
type UploadState struct {
	LocalPath    string       `json:"localPath"`
	FileHash     string       `json:"fileHash"`
	CollectionID int64        `json:"collectionID"`
	Status       UploadStatus `json:"status"`
	FileID       int64        `json:"fileID"`
	Error        string       `json:"error,omitempty"`
	UpdatedAt    int64        `json:"updatedAt"`
}

// FileMetadata contains metadata extracted from a file
type FileMetadata struct {
	Title            string    `json:"title"`
	CreationTime     int64     `json:"creationTime"`     // microseconds since epoch
	ModificationTime int64     `json:"modificationTime"` // microseconds since epoch
	FileType         int       `json:"fileType"`         // 0=image, 1=video, 2=live_photo
	Latitude         float64   `json:"latitude,omitempty"`
	Longitude        float64   `json:"longitude,omitempty"`
	Width            int       `json:"width,omitempty"`
	Height           int       `json:"height,omitempty"`
	FileSize         int64     `json:"-"` // Not in JSON metadata
	LocalCreated     time.Time `json:"-"` // Not in JSON metadata
	LocalModified    time.Time `json:"-"` // Not in JSON metadata
}

// EncryptedUploadData contains all encrypted data needed for upload
type EncryptedUploadData struct {
	// File encryption
	FileKey            []byte // Random 32-byte key (not uploaded, used for encryption)
	EncryptedFileKey   string // Encrypted with collection key
	KeyDecryptionNonce string // Nonce for file key encryption

	// Encrypted file data
	FileObjectKey        string // S3 key: {userID}/{uuid}
	FileDecryptionHeader string // Nonce for file decryption (base64)
	EncryptedFileSize    int64

	// Encrypted thumbnail data
	ThumbnailObjectKey        string
	ThumbnailDecryptionHeader string // Nonce for thumbnail decryption (base64)
	EncryptedThumbnailSize    int64

	// Encrypted metadata
	EncryptedMetadata        string
	MetadataDecryptionHeader string // Nonce for metadata decryption (base64)
}

// UploadConfig contains configuration for the uploader
type UploadConfig struct {
	Workers      int  // Number of concurrent upload workers
	ForceUpload  bool // Force upload even if duplicate exists
	CreateAlbum  bool // Create album if it doesn't exist
	ChunkSize    int64
	MultipartMin int64 // Minimum size for multipart upload (20MB)
}

// FileType constants
const (
	FileTypeImage     = 0
	FileTypeVideo     = 1
	FileTypeLivePhoto = 2
)
