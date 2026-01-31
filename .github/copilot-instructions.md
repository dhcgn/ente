# Copilot Coding Agent Instructions

## Repository Overview

This is a **standalone CLI tool repository**, forked from [ente-io/ente](https://github.com/ente-io/ente) and cleaned up to contain only the Go CLI with extended functionality.

### Original Purpose
The Ente CLI was developed by the Ente team for:
- Exporting photos/videos (backup functionality)
- Account management

### Extended Features (This Fork)
- ✅ **Upload**: Upload images with end-to-end encryption
- ✅ **Watch**: Monitor folders and auto-upload (3 modes: default, specified album, folder-as-album)
- ✅ **Deduplication**: Smart duplicate detection with album assignment

## Repository Structure

```
ente/
├── cli/                 # Main CLI tool (all development here)
│   ├── cmd/            # Commands: account, export, upload, watch
│   ├── internal/       # Internal packages (api, crypto)
│   ├── pkg/            # Public packages (uploader, watcher, model)
│   └── docs/           # Generated CLI docs
├── architecture/       # Ente architecture reference (from upstream)
└── .github/           # CI/CD workflows
```

## Development Commands

Always work from the `cli/` directory:

```sh
cd cli
```

### Build
```sh
go build -o bin/ente main.go
```

### Test
```sh
go test ./...              # All tests
go test ./pkg/uploader/... # Upload-specific
go test ./pkg/watcher/...  # Watch-specific
```

### Run
```sh
./bin/ente --help
./bin/ente export
./bin/ente upload photo.jpg --album "Vacation"
./bin/ente watch ~/Photos --folder-albums
```

### Regenerate Docs
```sh
go run main.go docs
```

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
- `thumbnail.go` - Thumbnail generation
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

## Watch Feature Architecture

Three modes for folder watching:
1. **Default**: All files → "CLI Uploads" album
2. **Specified** (`--album`): All files → user-specified album
3. **Folder-as-Album** (`--folder-albums`): Each subfolder → separate album

**Duplicate Handling:**
- If file already uploaded, don't re-upload
- Instead, add to target album (re-encrypt key with target collection key)
- Uses `/collections/add-files` API endpoint

**State Persistence:**
- BoltDB stores watch state and processed files
- Recovers on restart (graceful shutdown with Ctrl+C)

## Encryption Model

Maintains Ente's end-to-end encryption:
- Master key (user password-derived)
- Collection keys (per-album, encrypted with master key)
- File keys (per-file, encrypted with collection key)
- Server only handles encrypted blobs

See `architecture/` for detailed cryptography docs.

## Testing Notes

### Known Issue (Windows)
`TestResolvePath` in `cli/internal/promt_test.go` may fail on Windows due to drive letter paths. This is pre-existing and not related to upload/watch functionality.

### Testing Upload/Watch
```sh
cd cli
go test ./pkg/uploader/...
go test ./pkg/watcher/...
go test ./internal/crypto/...
```

## CI/CD

GitHub Actions workflow: `.github/workflows/cli-release.yml`
- Builds CLI for multiple platforms
- Runs tests
- Creates releases

## Upstream Reference

For the full Ente ecosystem (web, mobile, desktop, server):
https://github.com/ente-io/ente

This repository focuses solely on the CLI tool with extended upload and watch capabilities.

## Guidelines

1. **Stay focused on CLI**: All development in `cli/` directory
2. **Follow existing patterns**: Reuse components in `uploader/` and `watcher/`
3. **Preserve encryption**: Don't modify crypto without consulting `architecture/`
4. **Test thoroughly**: Unit tests for new features
5. **Document changes**: Update `cli/docs/` via `go run main.go docs`
6. **Maintain compatibility**: Uploaded files should work across all Ente clients

## Code Quality

Run these from the `cli/` directory:

```sh
go fmt ./...
go vet ./...
```

## Guidance for Coding Agents

- **Trust this file as primary reference** for this fork
- **Stay within the CLI**: Limit code changes to `cli/` directory
- **Follow existing patterns**: Reuse and extend existing components
- **Avoid crypto redesigns**: Consult `architecture/README.md` first
- **Be platform-aware**: Remember Windows path-testing quirk
- **Keep behavior predictable**: New features should be backward compatible

If you need information not covered here, consult `cli/README.md` and the files mentioned above first before widening your search.
