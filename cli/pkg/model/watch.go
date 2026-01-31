package model

// WatchMode represents the mode of operation for the folder watcher
type WatchMode int

const (
	WatchModeDefault WatchMode = iota // Upload to "CLI Uploads"
	WatchModeSpecified                // Upload to user-specified album
	WatchModeFolderAlbums             // Each subfolder becomes an album
)

func (m WatchMode) String() string {
	switch m {
	case WatchModeDefault:
		return "default"
	case WatchModeSpecified:
		return "specified"
	case WatchModeFolderAlbums:
		return "folder-albums"
	default:
		return "unknown"
	}
}

// WatchState stores the persistent state of a folder watcher
type WatchState struct {
	WatchPath     string    `json:"watchPath"`     // Absolute path being watched
	Mode          WatchMode `json:"mode"`          // Watch mode (default/specified/folder-albums)
	AlbumName     string    `json:"albumName"`     // For specified mode
	Workers       int       `json:"workers"`       // Number of concurrent upload workers
	DebounceMs    int       `json:"debounceMs"`    // Debounce delay in milliseconds
	StartedAt     int64     `json:"startedAt"`     // Unix timestamp (seconds)
	LastProcessed int64     `json:"lastProcessed"` // Unix timestamp (seconds)
}

// FileProcessStatus represents the processing status of a file
type FileProcessStatus int

const (
	StatusProcessing FileProcessStatus = iota
	StatusUploaded
	StatusDuplicate
	StatusFailed
)

func (s FileProcessStatus) String() string {
	switch s {
	case StatusProcessing:
		return "processing"
	case StatusUploaded:
		return "uploaded"
	case StatusDuplicate:
		return "duplicate"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// ProcessedFile tracks files that have been processed by the watcher
type ProcessedFile struct {
	FilePath     string            `json:"filePath"`     // Absolute path of the file
	FileHash     string            `json:"fileHash"`     // SHA256 hash
	FileID       int64             `json:"fileID"`       // Ente file ID (0 if not uploaded)
	CollectionID int64             `json:"collectionID"` // Collection/album ID
	ProcessedAt  int64             `json:"processedAt"`  // Unix timestamp (seconds)
	Status       FileProcessStatus `json:"status"`       // Processing status
	Error        string            `json:"error,omitempty"`
}

// FileHashMapping stores the mapping from file hash to file ID and collection ID
type FileHashMapping struct {
	FileID       int64 `json:"fileID"`
	CollectionID int64 `json:"collectionID"`
}
