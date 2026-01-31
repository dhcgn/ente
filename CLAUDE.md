# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Purpose

**This is a standalone CLI tool repository, forked and cleaned up from [ente-io/ente](https://github.com/ente-io/ente).**

### Original Development

The Ente CLI was originally developed by the Ente team for:
- Downloading photos/videos (export/backup functionality)
- Account management
- Basic file operations

### Extended Functionality (This Fork)

This fork extends the CLI with:
- ✅ **Image upload**: Upload single/multiple images with E2E encryption
- ✅ **Folder watching**: Monitor folders and auto-upload new images (3 modes)
- ✅ **Album management**: Organize uploads into collections
- ✅ **Deduplication**: Smart duplicate detection and album assignment

## Repository Structure

```
ente/
├── cli/                   # Main CLI tool (Go)
│   ├── cmd/              # Commands: account, export, upload, watch
│   ├── internal/         # Internal packages (api, crypto)
│   │   ├── api/         # Ente API client
│   │   └── crypto/      # Encryption utilities
│   ├── pkg/             # Public packages
│   │   ├── uploader/    # Upload pipeline and helpers
│   │   ├── watcher/     # Folder watching functionality
│   │   └── model/       # Data structures
│   └── docs/            # Generated documentation
├── architecture/         # Ente architecture docs (from upstream)
└── .github/             # CI/CD workflows
```

## CLI Development

### Prerequisites
- Go 1.20+ (tested with Go 1.25.5)
- FFmpeg (for thumbnail generation)

### Building
```bash
cd cli
go build -o bin/ente main.go
```

### Commands

#### Export (Original Feature)
```bash
./bin/ente export
```

#### Upload (Extended Feature)
```bash
./bin/ente upload photo.jpg --album "Vacation"
./bin/ente upload *.jpg -r --workers=8
```

#### Watch (Extended Feature)
```bash
# Default mode: upload to "CLI Uploads"
./bin/ente watch ~/Photos

# Specified album mode
./bin/ente watch ~/Photos --album="Family Photos"

# Folder-as-album mode (each subfolder = album)
./bin/ente watch ~/Photos --folder-albums
```

### Testing
```bash
cd cli
go test ./...
go test ./pkg/uploader/...  # Upload-specific
go test ./pkg/watcher/...   # Watch-specific
```

### Code Quality
```bash
go fmt ./...
go vet ./...
```

## Implementation Details

### Upload Pipeline
1. File discovery and hash computation
2. Deduplication check (local + remote)
3. Metadata extraction (EXIF)
4. Thumbnail generation (FFmpeg)
5. Encryption (file, thumbnail, metadata)
6. S3 upload (presigned URLs)
7. Finalization via Museum API

### Watch Feature (3 Modes)
1. **Default**: Upload all to "CLI Uploads" album
2. **Specified**: Upload all to user-specified album (`--album`)
3. **Folder-as-Album**: Each subfolder becomes separate album (`--folder-albums`)

**Key Files:**
- `cli/cmd/watch.go` - Command definition
- `cli/pkg/watcher/watcher.go` - Main orchestrator
- `cli/pkg/watcher/file_watcher.go` - fsnotify wrapper
- `cli/pkg/watcher/duplicate_handler.go` - Duplicate detection
- `cli/internal/api/collection_files.go` - API for album assignment

### Duplicate Handling
**Critical**: If file already uploaded, don't re-upload - ADD to target album instead.
- Uses file hash for deduplication
- Re-encrypts file key with target collection key
- Adds file to album via `/collections/add-files` API

## Key Components

### Commands (cli/cmd/)
- `upload.go` - Upload command (single/batch files)
- `watch.go` - Folder watching command (3 modes)
- `export.go` - Export/backup command (original)
- `account.go` - Account management

### Upload Pipeline (cli/pkg/uploader/)
- `uploader.go` - Main upload orchestrator
- `file_processor.go` - File discovery and metadata
- `encryptor.go` - Client-side encryption
- `thumbnail.go` - Thumbnail generation (FFmpeg)
- `s3_uploader.go` - S3 upload handling
- `deduplicator.go` - Duplicate detection
- `upload_helpers.go` - Single-file upload helpers

### Watch Feature (cli/pkg/watcher/)
- `watcher.go` - Main watcher orchestrator (3 modes)
- `file_watcher.go` - fsnotify wrapper (recursive watching)
- `debounce.go` - File write completion detection
- `duplicate_handler.go` - Duplicate detection and album assignment

### API Client (cli/internal/api/)
- `client.go` - HTTP client with auth
- `upload_api.go` - Upload URL requests
- `files.go` - File finalization
- `collection.go` - Collection/album operations
- `collection_files.go` - Add files to collections

### Encryption (cli/internal/crypto/)
- `crypto.go` - Core encryption primitives
- `crypto_libsodium.go` - Libsodium bindings
- `stream.go` - Stream cipher for large files

## Encryption & Security

All uploads maintain Ente's end-to-end encryption model:
- Files encrypted client-side before upload
- Per-file keys encrypted with collection keys
- Collection keys encrypted with master key
- Server only sees encrypted blobs

See `architecture/` for detailed cryptography documentation.

## Testing Notes

### Known Issue (Windows)
`TestResolvePath` in `cli/internal/promt_test.go` may fail on Windows due to drive letter paths. This is pre-existing and not related to upload/watch functionality.

### Testing Upload/Watch
```bash
cd cli
go test ./pkg/uploader/...
go test ./pkg/watcher/...
go test ./internal/crypto/...
```

## Upstream Reference

For the full Ente ecosystem (web/mobile/desktop/server), see:
https://github.com/ente-io/ente

## Commit Guidelines

- Keep messages concise (subject line under 72 chars)
- No emojis, no promotional text
- Format: Single sentence describing the change
- Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>

Example:
```
Add folder watch functionality to Ente CLI

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
```

## Guidelines for Development

1. **Stay focused on CLI**: All development in `cli/` directory
2. **Follow existing patterns**: Reuse components in `uploader/` and `watcher/`
3. **Preserve encryption**: Don't modify crypto without consulting `architecture/`
4. **Test thoroughly**: Unit tests for new features
5. **Document changes**: Update `cli/docs/` via `go run main.go docs`
6. **Maintain compatibility**: Uploaded files should work across all Ente clients

## External Documentation

- Full Ente documentation: https://ente.io/help
- Self-hosting guide: https://ente.io/help/self-hosting
- Cryptography audit: https://ente.io/blog/cryptography-audit/
- Architecture: https://ente.io/architecture
