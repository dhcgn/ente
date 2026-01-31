# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Fork Purpose

**This is a fork of the official ente-io/ente repository.**

The primary goal of this fork is to implement **image upload functionality for the Ente CLI** (tracked in [issue #1](https://github.com/dhcgn/ente/issues/1)).

### Feature Overview

The CLI currently supports exporting data and managing accounts, but lacks upload capabilities. This fork adds:

- **File uploads**: Upload single or multiple images from local filesystem to Ente Photos
- **Album management**: Organize uploads into collections/albums
- **Deduplication**: Detect and skip duplicate files via content hash comparison
- **End-to-end encryption**: Maintain security standards (libsodium, ChaCha20-Poly1305)
- **Progress tracking**: Provide meaningful CLI feedback during upload operations

### Implementation Focus

When working on upload functionality:

1. Follow the upstream encryption/upload flow:
   - Compute file hash for deduplication
   - Request upload URL from server API
   - Encrypt file/thumbnail/metadata client-side
   - Upload encrypted data
   - Finalize via `/files` API endpoint

2. Primary development area: `cli/` directory (Go codebase)
3. Reference existing mobile/web upload implementations for patterns
4. Leverage existing CLI infrastructure (BoltDB for state, libsodium bindings)
5. Ensure compatibility with upstream Ente server (Museum)

## About Ente

Ente is a fully open source, end-to-end encrypted platform for storing data in the cloud. This monorepo contains:

- **Ente Photos**: A fully-featured photo management and sharing app
- **Ente Auth**: A 2FA authenticator app with cloud backups
- Client apps for iOS, Android, Web, Desktop (Linux/macOS/Windows), and CLI
- **Museum**: The API server that powers all apps (Go + PostgreSQL)

All data is end-to-end encrypted before leaving the user's device using libsodium. The server (museum) is data-agnostic and handles encrypted blobs. See `architecture/README.md` for detailed cryptography documentation.

## Repository Structure

```
ente/
├── web/                    # Web apps (Next.js + React + TypeScript)
│   ├── apps/              # Photos, Auth, Cast, Accounts, Payments, etc.
│   └── packages/          # Shared code (base, gallery, media, utils)
├── mobile/                # Mobile apps (Flutter + Dart)
│   ├── apps/              # Photos, Auth, Locker
│   └── packages/          # Shared packages
├── desktop/               # Desktop app (Electron wrapping web/photos)
├── server/                # Museum API server (Go + PostgreSQL)
├── cli/                   # Go CLI for exports and utilities
├── rust/                  # Shared Rust code
│   ├── core/              # Pure Rust business logic (ente-core)
│   └── cli/               # Rust CLI (work in progress)
├── infra/                 # Infrastructure and hosting utilities
└── architecture/          # Cryptography and architecture documentation
```

Each major component (web/, mobile/, server/, desktop/) has its own README.md and may have component-specific CLAUDE.md files with additional context.

## Common Development Commands

### Web (yarn workspaces, uses Yarn v1.22.22)

```bash
cd web/

# Install dependencies
yarn install

# Development servers
yarn dev              # Photos app (port 3000)
yarn dev:auth         # Auth app (port 3003)
yarn dev:accounts     # Accounts app (port 3001)
yarn dev:cast         # Cast app (port 3004)
yarn dev:embed        # Embed app (port 3006)
yarn dev:share        # Share app (port 3005)

# Production builds
yarn build            # Photos app
yarn build:auth       # Auth app
yarn build:wasm       # Build WASM package from rust/core

# Code quality (only run when explicitly requested or before commits)
yarn lint             # Check formatting, linting, and TypeScript
yarn lint-fix         # Auto-fix issues
yarn test             # Run tests (WASM package)
```

### Mobile (Flutter + Melos for monorepo management)

```bash
cd mobile/

# Setup - install Melos globally first
dart pub global activate melos

# Bootstrap workspace (links packages, runs pub get everywhere)
melos bootstrap

# Generate Rust bindings (required after Rust code changes)
melos run codegen:rust          # All packages
melos run codegen:rust:photos   # Photos app only

# Run apps
melos run:photos:apk      # Run Photos app
melos run:auth:apk        # Run Auth app

# Build apps
melos build:photos:apk    # Build Photos APK

# Clean
melos clean:photos        # Clean Photos app
melos clean:all           # Clean all packages

# Code quality
cd apps/photos/           # Navigate to specific app
dart format .             # Format code
flutter analyze           # Static analysis
flutter test              # Run tests
```

Alternatively, use Flutter directly:

```bash
cd mobile/apps/photos/
flutter run -t lib/main.dart --flavor independent
flutter build apk --release --flavor independent
```

### Server (Go + Docker)

```bash
cd server/

# Quick start with Docker (easiest)
docker compose up --build

# Or use pre-built images
sh -c "$(curl -fsSL https://raw.githubusercontent.com/ente-io/ente/main/server/quickstart.sh)"

# Development without Docker
export ENTE_DB_USER=postgres
go run cmd/museum/main.go

# With live reload (requires air: github.com/cosmtrek/air)
air

# Testing
ENV="test" go test -v ./pkg/...

# Code quality
go fmt ./...
go vet ./...

# Connect web app to local server
cd ../web/
NEXT_PUBLIC_ENTE_ENDPOINT=http://localhost:8080 yarn dev

# Connect mobile app to local server
cd ../mobile/apps/photos/
flutter run --dart-define=endpoint=http://localhost:8080
```

### Desktop (Electron + Next.js)

```bash
cd desktop/

# Install dependencies
yarn install

# Development (hot reload for renderer)
yarn dev

# Build for current platform
yarn build

# Quick development build
yarn build:quick

# Code quality
yarn lint             # Check formatting, linting, TypeScript
yarn lint-fix         # Auto-fix issues
```

### CLI (Go) ⭐ PRIMARY DEVELOPMENT AREA FOR UPLOAD FEATURE

```bash
cd cli/

# Build
go build -o "bin/ente" main.go

# Build release binaries
./release.sh

# Run existing commands
./bin/ente --help
./bin/ente account add
./bin/ente export

# Upload feature (in development - see issue #1)
# When implemented, commands will be:
# ./bin/ente upload <file-path>
# ./bin/ente upload --album "Album Name" <file-path>

# Update documentation
go run main.go docs

# Testing
go test ./...
go test -v ./pkg/upload/...  # Upload-specific tests when implemented
```

### Rust

```bash
# Core library
cd rust/core/
cargo fmt               # Format
cargo clippy            # Lint
cargo build             # Build
cargo test              # Test

# CLI
cd rust/cli/
cargo run -- --help

# WASM (for web)
cd web/packages/wasm/
yarn build              # Runs wasm-pack build

# Mobile bindings (Flutter Rust Bridge)
cd mobile/packages/rust/
flutter_rust_bridge_codegen generate        # Generate Dart bindings

cd mobile/apps/photos/
flutter_rust_bridge_codegen generate        # Photos-specific bindings
```

### Comprehensive testing

```bash
# From repository root - test all mobile packages
./test-all-packages.sh
```

## Architecture Highlights

### End-to-End Encryption

All user data is encrypted client-side before upload:
- **Master Key**: Generated on device, encrypted with key derived from user password
- **Collection Keys**: Per-album/folder keys, encrypted with master key
- **File Keys**: Per-file keys, encrypted with collection keys
- **Key Pairs**: Public/private key pairs for sharing (sealed boxes)

See `architecture/README.md` for complete cryptography documentation including key derivation (Argon2), symmetric encryption (XSalsa20 + Poly1305), and asymmetric encryption (X25519).

### Web Architecture

- **Monorepo**: Yarn workspaces with multiple Next.js apps
- **Shared Packages**: Common code in `web/packages/` (base, gallery, media, utils)
- **Technology**: Next.js (static generation), React, TypeScript, Material-UI (MUI), Emotion
- **WASM Integration**: Rust core compiled to WASM via wasm-bindgen for crypto operations

### Mobile Architecture

- **Monorepo**: Melos manages multiple Flutter apps and shared packages
- **Service Layer**: 28+ specialized services (collections, sync, search, ML, etc.)
- **Rust Integration**: Flutter Rust Bridge for performance-critical operations
- **Database**: SQLite via sqlite_async
- **ML Features**: Face recognition, semantic search, similarity detection (ONNX runtime)

### Server Architecture (Museum)

- **Language**: Go (single statically-compiled binary)
- **Database**: PostgreSQL
- **Storage**: S3-compatible (supports any S3-compatible storage)
- **Stateless**: Server only handles encrypted blobs and metadata
- **Docker**: Fully containerized, easy to self-host

### Rust Integration

```
rust/core (ente-core)           # Pure Rust, no FFI
    ├── used by CLI
    ├── compiled to WASM via wasm-bindgen (web/packages/wasm)
    └── wrapped with Flutter Rust Bridge (mobile/packages/rust)
```

## Commit Guidelines

- Keep messages concise (subject line under 72 chars)
- No emojis
- No promotional text or links
- Format: Single sentence describing the change
- Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>

Example:
```
Add support for HEIC image format in gallery viewer

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
```

## Important Notes

### Running Code Quality Checks

- **Web**: Only run `yarn lint` when explicitly requested or before commits (not after every file modification)
- **Mobile**: Always run `dart format .` and `flutter analyze` before commits - CI will fail otherwise
- **Server**: Use standard Go tools (`go fmt`, `go vet`)
- **Desktop**: Run `yarn lint` before commits

### Package Management

- **Web/Desktop**: Yarn v1.22.22 (specified in package.json)
- **Mobile**: Flutter packages + Melos for workspace management
- **Server/CLI**: Go modules
- **Rust**: Cargo

### Monorepo Navigation

- Web apps share code via `packages/` - check for existing components before creating new ones
- Mobile apps share code via `mobile/packages/` - Photos app also has app-specific plugins in `mobile/apps/photos/plugins/`
- When modifying shared packages, consider impact on all consuming apps

### Self-Hosting

Museum is designed for easy self-hosting. Everything needed is in `server/`. Docker Compose provided for quick setup with PostgreSQL and MinIO (S3-compatible storage).

### Security

- This is security-sensitive code - all changes affecting cryptography should be carefully reviewed
- Never compromise on end-to-end encryption guarantees
- Server should remain data-agnostic (only handles encrypted blobs)
- Follow existing patterns for key management

### Testing

- Web: Tests primarily in WASM package
- Mobile: Run `flutter test` in relevant app/package
- Server: Go tests in `pkg/` with separate test database
- Always test encryption/decryption flows when modifying crypto code

### Working on Upload Feature (Fork-Specific)

When implementing CLI upload functionality:

1. **Study existing implementations first**:
   - Mobile upload logic: `mobile/apps/photos/lib/services/upload_service.dart`
   - Web upload logic: Check `web/apps/photos/src/services/upload/` or similar
   - Server API endpoints: `server/pkg/controller/filedata/` and `server/ente/file.go`

2. **Key files to reference in CLI**:
   - Account management: `cli/internal/api/auth.go`
   - Encryption patterns: Look for existing crypto usage in CLI
   - Configuration: `cli/pkg/model/` for data structures

3. **Required functionality**:
   - File hashing for deduplication (SHA256 or similar)
   - E2E encryption before upload (ChaCha20-Poly1305 via libsodium)
   - Thumbnail generation for images
   - Metadata encryption (EXIF data)
   - Progress reporting for large uploads
   - Album/collection assignment

4. **Testing requirements**:
   - Unit tests for deduplication logic
   - Integration tests with local Museum instance
   - Encryption/decryption round-trip tests
   - Test with various image formats (JPEG, PNG, HEIC, etc.)

5. **Compatibility**: Ensure uploaded files can be viewed/downloaded by official mobile/web/desktop clients

## External Documentation

- Full documentation: https://ente.io/help
- Self-hosting guide: https://ente.io/help/self-hosting
- Cryptography audit: https://ente.io/blog/cryptography-audit/
- Architecture: https://ente.io/architecture
- Reliability: https://ente.io/reliability
