# Copilot Coding Agent Instructions

## Scope and Purpose

- This fork of [ente-io/ente](https://github.com/ente-io/ente) exists **only** to extend the Go CLI under [cli](cli) with encrypted media upload (photos, videos) for Ente Photos.
- Treat all other subprojects (web, mobile, desktop, server, rust, docs, infra) as read-only context; **do not** modify or build them unless explicitly instructed.
- The codebase is security-sensitive. Follow existing cryptography and upload patterns; consult [architecture/README.md](architecture/README.md) before changing anything related to keys or encryption.

Always start by changing into the CLI directory:

```sh
cd cli
```

All build, test, and run commands in this file assume that working directory.

## CLI Layout (upload-focused)

High-level CLI structure (see [cli](cli)):

- Entry point: [cli/main.go](cli/main.go)
  - Wires Cobra root command and subcommands.
- Commands: [cli/cmd](cli/cmd)
  - Root: [cli/cmd/root.go](cli/cmd/root.go)
  - Version: [cli/cmd/version.go](cli/cmd/version.go)
  - Accounts/admin/export: [cli/cmd/account.go](cli/cmd/account.go), [cli/cmd/admin.go](cli/cmd/admin.go), [cli/cmd/export.go](cli/cmd/export.go)
  - **Upload command entry**: [cli/cmd/upload.go](cli/cmd/upload.go) – argument parsing, flags, and high-level control for upload.
- Core CLI logic: [cli/pkg](cli/pkg)
  - CLI plumbing: [cli/pkg/cli.go](cli/pkg/cli.go)
  - Sign-in, store, and sync: [cli/pkg/sign_in.go](cli/pkg/sign_in.go), [cli/pkg/store.go](cli/pkg/store.go), [cli/pkg/remote_sync.go](cli/pkg/remote_sync.go)
  - Legacy export/disk operations: [cli/pkg/disk.go](cli/pkg/disk.go), [cli/pkg/download.go](cli/pkg/download.go)
  - **Upload orchestration**: [cli/pkg/upload.go](cli/pkg/upload.go) – ties together uploader components.
- Upload domain and helpers: [cli/pkg/uploader](cli/pkg/uploader)
  - [cli/pkg/uploader/file_processor.go](cli/pkg/uploader/file_processor.go) – walks local files, computes hashes, gathers metadata.
  - [cli/pkg/uploader/deduplicator.go](cli/pkg/uploader/deduplicator.go) – deduplication using content hashes and remote state.
  - [cli/pkg/uploader/encryptor.go](cli/pkg/uploader/encryptor.go) – file/thumbnail/metadata encryption using existing crypto primitives.
  - [cli/pkg/uploader/thumbnail.go](cli/pkg/uploader/thumbnail.go) – thumbnail generation.
  - [cli/pkg/uploader/s3_uploader.go](cli/pkg/uploader/s3_uploader.go) – upload to presigned S3-compatible URLs.
  - [cli/pkg/uploader/progress.go](cli/pkg/uploader/progress.go) – progress tracking and reporting.
  - [cli/pkg/uploader/uploader.go](cli/pkg/uploader/uploader.go) – high-level upload pipeline.
- Upload models and shared types: [cli/pkg/model](cli/pkg/model)
  - [cli/pkg/model/upload.go](cli/pkg/model/upload.go) – upload-related data structures (hashes, collection/album info, API payloads).
  - Other models for accounts, exports, and errors live alongside.
- API client and upload endpoints: [cli/internal/api](cli/internal/api)
  - [cli/internal/api/client.go](cli/internal/api/client.go) – HTTP client configuration and auth token handling.
  - [cli/internal/api/files.go](cli/internal/api/files.go), [cli/internal/api/upload_api.go](cli/internal/api/upload_api.go) – request upload URLs, finalize uploads, talk to Museum.
  - Related enums and types in [cli/internal/api/enums.go](cli/internal/api/enums.go) and [cli/internal/api/file_type.go](cli/internal/api/file_type.go).
- Cryptography and key handling: [cli/internal/crypto](cli/internal/crypto)
  - [cli/internal/crypto/crypto.go](cli/internal/crypto/crypto.go), [cli/internal/crypto/crypto_libsodium.go](cli/internal/crypto/crypto_libsodium.go), [cli/internal/crypto/stream.go](cli/internal/crypto/stream.go)
  - Follow these patterns (and architecture docs) for any new encryption logic.
- Configuration & secrets:
  - Example CLI configuration: [cli/config.yaml.example](cli/config.yaml.example)
  - Secrets and key holder: [cli/pkg/secrets](cli/pkg/secrets)
- CLI docs:
  - Top-level: [cli/README.md](cli/README.md)
  - Generated command docs: [cli/docs/generated](cli/docs/generated)

## Build and Run (CLI only)

Prerequisites:

- Go toolchain (modules-enabled). Go 1.20+ works well (repository tested with Go 1.25.5 on Windows).

Always run these from [cli](cli):

Build the CLI binary:

```sh
go build -o "bin/ente" main.go
```

- Output: `bin/ente` (self-contained CLI executable).

Run the CLI (after build):

```sh
bin/ente --help
bin/ente upload --help
```

If you install the binary into your PATH, you can use `ente` instead of `bin/ente`.

Regenerate CLI docs when changing command help or flags:

```sh
go run main.go docs
```

This updates Markdown files under [cli/docs/generated](cli/docs/generated).

## Tests and Known Issues (Windows)

To run all Go tests for the CLI:

```sh
go test ./...
```

On Windows, there is a **known, pre-existing failure** in [cli/internal/promt_test.go](cli/internal/promt_test.go) for `TestResolvePath`:

- Example failure (observed): `Expected "\\test", got "H:\\test"`.
- This is due to Windows drive-letter paths vs. expected UNC-style paths.
- Treat this as an environmental quirk unless you are changing `ResolvePath` in [cli/internal/promt.go](cli/internal/promt.go).

When working on upload functionality and unrelated to path handling, prefer targeted tests to avoid noise from that failure:

```sh
go test ./pkg/...
go test ./pkg/uploader/...
go test ./internal/crypto/...
```

If you intentionally change path resolution logic, you should:

- Update [cli/internal/promt.go](cli/internal/promt.go) and [cli/internal/promt_test.go](cli/internal/promt_test.go).
- Ensure `go test ./internal/...` passes on the target platform (including Windows, if relevant).

## Upload Feature Architecture (CLI)

The upload pipeline in this fork follows the same high-level flow as official Ente clients:

1. Discover candidate files and compute hashes for deduplication.
2. Request upload URLs and metadata slots from the Museum API.
3. Encrypt file bytes, thumbnails, and metadata **locally**.
4. Upload encrypted blobs to S3-compatible storage using presigned URLs.
5. Finalize via Museum `/files` endpoints and update local state.

Key places to make changes or add behavior:

- **Command surface** – user-facing CLI options and arguments:
  - [cli/cmd/upload.go](cli/cmd/upload.go)
    - Add or adjust flags (e.g., album selection, concurrency, dry-run) here.
    - Keep help text in sync with generated docs (`go run main.go docs`).
- **Orchestration and business logic**:
  - [cli/pkg/upload.go](cli/pkg/upload.go)
    - Central entry point from the command to the upload pipeline.
    - Coordinates account selection, configuration, and uploader invocation.
  - [cli/pkg/uploader/uploader.go](cli/pkg/uploader/uploader.go)
    - Controls the end-to-end pipeline (hashing, dedupe, encrypt, upload, finalize).
- **Deduplication and file discovery**:
  - [cli/pkg/uploader/file_processor.go](cli/pkg/uploader/file_processor.go)
  - [cli/pkg/uploader/deduplicator.go](cli/pkg/uploader/deduplicator.go)
  - [cli/pkg/model/upload.go](cli/pkg/model/upload.go)
    - Extend these if you need new metadata for deduping or selection.
- **Encryption and thumbnails**:
  - [cli/internal/crypto](cli/internal/crypto) (core primitives, libsodium usage).
  - [cli/pkg/uploader/encryptor.go](cli/pkg/uploader/encryptor.go)
  - [cli/pkg/uploader/thumbnail.go](cli/pkg/uploader/thumbnail.go)
    - When in doubt, mirror existing patterns; do **not** invent new crypto schemes.
- **Remote API integration**:
  - [cli/internal/api/upload_api.go](cli/internal/api/upload_api.go)
  - [cli/internal/api/files.go](cli/internal/api/files.go)
  - [cli/internal/api/models](cli/internal/api/models)
    - Use these to talk to the Museum server for upload URLs, metadata, and finalization.

Keep the CLI upload behavior compatible with existing Ente clients so that uploaded media is fully usable across web, mobile, and desktop.

## CI and Validation (CLI-focused)

- The relevant GitHub Actions workflow is [ .github/workflows/cli-release.yml ](.github/workflows/cli-release.yml), which builds and tests the CLI for release.
- Before opening a pull request, at minimum:
  - Run `go test ./pkg/...` and any more specific packages you touched (e.g., `./pkg/uploader`, `./internal/crypto`).
  - Optionally run `go test ./...` and note the known Windows path test issue if it appears.

You may also use standard Go tools locally when touching CLI code:

```sh
go fmt ./...
go vet ./...
```

Run them only from [cli](cli); do not attempt to vet or fmt the entire monorepo in one go.

## Guidance for Future Coding Agents

- **Trust this file as the primary onboarding reference for this fork.** Only fall back to broader searches (`grep`, `go list`, browsing other subprojects) when information here is clearly insufficient or contradictory.
- **Stay within the CLI.** Limit code changes to [cli](cli) (and occasionally supporting CI/docs for the CLI) unless the user explicitly asks otherwise.
- **Follow existing patterns.** For uploads, reuse and extend the existing components in [cli/pkg/uploader](cli/pkg/uploader), [cli/pkg/model](cli/pkg/model), and [cli/internal/api](cli/internal/api) rather than introducing parallel abstractions.
- **Avoid crypto redesigns.** If you must touch encryption or key management, read [architecture/README.md](architecture/README.md) and study [cli/internal/crypto](cli/internal/crypto) first; prefer small, local changes over new primitives.
- **Be platform-aware.** Remember the known Windows path-testing quirk in `TestResolvePath`; do not treat that alone as a regression in your upload changes.
- **Keep behavior predictable.** New upload-related flags or behaviors should remain backward compatible and should be reflected in [cli/docs/generated](cli/docs/generated) via `go run main.go docs`.

If you need information that is not covered here, consult [cli/README.md](cli/README.md) and the files mentioned above first; only then widen your search to other parts of the repository.