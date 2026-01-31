# Ente CLI

Standalone command-line tool for Ente Photos with upload and folder watching capabilities.

## About This Fork

This is a fork of the official [ente-io/ente](https://github.com/ente-io/ente) repository, cleaned up to contain only the CLI tool with extended functionality.

### Original Purpose

The Ente CLI was originally developed by the Ente team for:
- Exporting photos/videos to create local backups
- Account management and basic file operations

### Extended Features (This Fork)

This fork adds:
- **Upload**: Upload images to Ente Photos with end-to-end encryption
- **Watch**: Automatically upload new images from monitored folders
- Support for albums, deduplication, and smart duplicate detection

## Features

### Export (Original Feature)
- Download all your photos and videos from Ente Photos
- Create local backups with original quality
- Supports incremental exports

### Upload (Extended Feature)
- Upload single or multiple images with end-to-end encryption
- Batch upload with configurable concurrency
- Smart deduplication (skip already-uploaded files)
- Album organization and management
- Progress tracking for large uploads

### Watch (Extended Feature)
- Monitor folders for new images and auto-upload
- Three modes:
  - **Default**: Upload all to "CLI Uploads" album
  - **Specified Album** (`--album`): Upload all to user-specified album
  - **Folder-as-Album** (`--folder-albums`): Each subfolder becomes a separate album
- Duplicate detection with automatic album assignment
- Graceful shutdown and state recovery

### Other Features
- Account management (add, list, update)
- Auth token decryption
- Admin functions for self-hosted instances

### Security
- End-to-end encryption using Ente's proven security model
- All files encrypted client-side before upload
- Per-file encryption keys, encrypted with collection keys
- Server only sees encrypted blobs
- Compatible with all official Ente clients (web, mobile, desktop)

## Installation

### Download Pre-Built Binaries

The easiest way is to download a pre-built binary from the [GitHub releases](https://github.com/ente-io/ente/releases?q=tag%3Acli-v0).

### Prerequisites for Building from Source
- Go 1.20 or higher
- FFmpeg (for thumbnail generation)

### Build Release Binaries

```bash
cd cli
./release.sh
```

### Build from Source

```bash
cd cli
go build -o bin/ente main.go
```

The generated binaries are standalone, static binaries with no dependencies. You can run them directly, or put them somewhere in your PATH.

There is also an option to use [Docker](#docker).

## Usage

Run the help command to see all available commands:

```bash
ente --help
```

### Account Management

If you wish, you can add multiple accounts (your own and that of your family members) and export all data using this tool.

#### Add an account

```bash
ente account add
```

> [!NOTE]
>
> `ente account add` does not create new accounts, it just adds pre-existing accounts to the list of accounts that the CLI knows about so that you can use them for other actions.

#### List accounts

```bash
ente account list
```

#### Change export directory

```bash
ente account update --app auth/photos --email email@domain.com --dir ~/photos
```

### Export (Backup)

Export all photos and videos:

```bash
ente export
```

### Upload

#### Upload a single image

```bash
ente upload photo.jpg
```

#### Upload to a specific album

```bash
ente upload photo.jpg --album "Vacation 2024"
```

#### Upload multiple files

```bash
ente upload *.jpg --album "Summer"
```

#### Upload recursively with custom concurrency

```bash
ente upload /path/to/photos -r --workers=8
```

### Watch Folders

#### Watch a folder (default "CLI Uploads" album)

```bash
ente watch ~/Photos
```

#### Watch and upload to a specific album

```bash
ente watch ~/Photos --album "Family Photos"
```

#### Watch with folder-as-album mode

Each subfolder becomes a separate album:

```bash
ente watch ~/Photos --folder-albums
```

Stop watching by pressing `Ctrl+C` for graceful shutdown with state preservation.

### CLI Documentation

You can view more CLI documentation at [cli/docs/generated/ente.md](cli/docs/generated/ente.md).

To update the docs, run:

```bash
cd cli
go run main.go docs
```

## Docker

If you fancy Docker, you can also run the CLI within a container.

### Configure

Modify the `docker-compose.yml` and add volume. `cli-data` volume is mandatory, you can add more volumes for your export directory.

Build and run the container in detached mode:

```bash
docker-compose up -d --build
```

Note that [BuildKit](https://docs.docker.com/go/buildkit/) is needed to build this image. If you face this issue, a quick fix is to add `DOCKER_BUILDKIT=1` in front of the build command.

Execute commands in the container:

```bash
docker-compose exec ente-cli /bin/sh -c "./ente-cli version"
docker-compose exec ente-cli /bin/sh -c "./ente-cli account add"
docker-compose exec ente-cli /bin/sh -c "./ente-cli upload photo.jpg"
docker-compose exec ente-cli /bin/sh -c "./ente-cli watch /path/to/photos"
```

## Self-Hosting

For self-hosting configuration, see [cli/docs/selfhost.md](cli/docs/selfhost.md).

## Architecture

This repository includes the `architecture/` directory from the upstream Ente project, which contains detailed documentation on:
- End-to-end encryption model
- Key derivation and management
- Cryptographic primitives (libsodium, ChaCha20-Poly1305)
- Security audits and specifications

See `architecture/README.md` for complete details.

### Upload Pipeline

1. File discovery and hash computation
2. Deduplication check (local + remote)
3. Metadata extraction (EXIF)
4. Thumbnail generation (FFmpeg)
5. Client-side encryption (file, thumbnail, metadata)
6. S3 upload to presigned URLs
7. Finalization via Museum API

### Duplicate Handling

If a file is already uploaded:
- File is NOT re-uploaded (saves bandwidth and time)
- File is added to the target album (re-encrypts file key with collection key)
- Uses content hash for reliable deduplication

## Upstream Project

For the full Ente ecosystem including web, mobile, desktop apps, and the Museum API server, see:
- **Main repository**: https://github.com/ente-io/ente
- **Website**: https://ente.io
- **Documentation**: https://ente.io/help

## Development

### Running Tests

```bash
cd cli
go test ./...
```

Test specific packages:
```bash
go test ./pkg/uploader/...
go test ./pkg/watcher/...
```

### Code Quality

```bash
cd cli
go fmt ./...
go vet ./...
```

### Regenerate Documentation

```bash
cd cli
go run main.go docs
```

## Contributing

Contributions are welcome! When submitting pull requests:
- Keep commit messages concise (under 72 chars)
- Test your changes thoroughly
- Ensure `go fmt` and `go vet` pass
- Update documentation if adding new features

## License

This project inherits the license from the upstream [ente-io/ente](https://github.com/ente-io/ente) repository. See the LICENSE file for details.

## Security

If you discover a security vulnerability, please report it responsibly by emailing security@ente.io or using [this link](https://github.com/ente-io/ente/security/advisories/new).

## Support

For questions about the upstream Ente platform, see:
- [Support Guide](https://github.com/ente-io/ente/blob/main/SUPPORT.md)
- [Community](https://ente.io/about#community)
- [Discord](https://discord.gg/z2YVKkycX3)

For issues specific to this fork's upload/watch functionality, please open an issue in this repository.
