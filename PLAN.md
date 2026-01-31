# Fix Plan: "TypeError: cipher is too short" Bug

## Problem Statement

When uploading images via the CLI to Ente, files are uploaded successfully but fail to display in the web interface with the error:

```
[error] Failed to obtain renderableSourceURLs: TypeError: cipher is too short
at Object.t0 [as crypto_secretstream_xchacha20poly1305_pull]
```

Thumbnails render correctly, but full-size images cannot be decrypted.

## Root Cause Analysis - UPDATED

After thorough analysis of both the CLI upload code and the web decryption code, I've identified that the CLI implementation is **theoretically correct** but there's a subtle bug causing the issue.

### Current CLI Implementation

In `cli/pkg/uploader/encryptor.go` (lines 83-86):

```go
// NOTE: Do NOT write header to output file
// The header must be stored separately in metadata (decryptionHeader field)
// Only encrypted chunks are written to the file uploaded to S3
```

The CLI:
1. Generates a 24-byte encryption header (nonce) via `crypto.NewEncryptor(key)`
2. Does NOT write this header to the encrypted file
3. Only writes encrypted chunks to the file
4. Stores the header separately in `FileDecryptionHeader` metadata field

### Web Client Decryption Flow

Confirmed in `web/packages/base/crypto/libsodium.ts:533-563`:

```typescript
export const decryptStreamBytes = async (
    { encryptedData, decryptionHeader }: EncryptedFile,
    key: BytesOrB64,
): Promise<Uint8Array> => {
    // Initialize decryption with header FROM METADATA
    const pullState = sodium.crypto_secretstream_xchacha20poly1305_init_pull(
        await fromB64(decryptionHeader),  // Uses header from metadata
        await bytes(key),
    );

    // Decrypt the encryptedData (just the chunks, NO header prepended)
    const buffer = encryptedData.slice(bytesRead, bytesRead + chunkSize);
    const pullResult = sodium.crypto_secretstream_xchacha20poly1305_pull(
        pullState,
        buffer,  // Just the encrypted chunks
    );
}
```

**Conclusion**: The web client expects:
- File in S3: Only encrypted chunks (NO header prepended)
- Header: Separately in metadata's `decryptionHeader` field

This matches the CLI implementation! So why is it failing?

### The Real Bug - Identified

The error "cipher is too short" occurs at `crypto_secretstream_xchacha20poly1305_pull`. Looking at our CLI crypto implementation at `cli/internal/crypto/stream.go:273-275`:

```go
if cipherLen < XChaCha20Poly1305IetfABYTES {
    return nil, 0, invalidInput  // This is where "cipher is too short" comes from
}
```

This means each encrypted chunk must be at least 17 bytes (1 tag + 16 MAC). The issue likely occurs when:

1. **Empty or very short final chunks** - If the CLI produces a chunk that's too short
2. **Incorrect chunk boundaries** - The decryption tries to read chunks but encounters data that doesn't align properly
3. **Missing final tag** - The stream encryption requires a final chunk with TAG_FINAL

### Investigation Needed

I need to verify the EXACT data being sent in the API request vs what's stored in S3. The debug output in `uploader.go:429-431` shows the API request, but I need to compare:

1. What `decryptionHeader` value is sent to the API
2. What size is reported for the encrypted file
3. Whether the encrypted file actually contains the final TAG chunk
4. Compare CLI-produced encrypted file with web-produced encrypted file byte-by-byte

## Hypotheses

### Hypothesis 1: Header Must Be Prepended (Most Likely)

The web client's decryption expects the file data to include the header as the first 24 bytes, even though the header is also stored in metadata. When decrypting:

```javascript
// Pseudo-code of what might be happening
const data = await downloadFile(url);  // Downloads from S3
const header = data.slice(0, 24);       // Extracts header from first 24 bytes
const cipher = data.slice(24);          // Remaining is encrypted data
decrypt(cipher, header);
```

**Evidence supporting this:**
- The HAR file (according to the agent's analysis) shows files with 24-byte headers prepended
- Error says "cipher is too short" suggesting missing bytes at the beginning

**Fix**: Modify `cli/pkg/uploader/encryptor.go` to write the header before encrypted chunks.

### Hypothesis 2: Metadata Handling Issue

The CLI stores the header in metadata correctly, but there's a mismatch in:
- Base64 encoding format
- Field name or structure in the API request
- The way the header bytes are encoded

**Evidence against this:**
- Thumbnails work fine (same encryption method)
- Debug output shows metadata is sent correctly

### Hypothesis 3: Empty Final Chunk Issue

For files that are exact multiples of the buffer size (4MB), the CLI writes an empty final chunk with just the TAG_FINAL marker (17 bytes: 1 tag + 16 MAC). This might cause issues if the web client doesn't expect it.

**Evidence against this:**
- The encryption logic looks correct for handling final chunks
- This is standard practice for stream ciphers

## Implementation Plan

### Phase 1: Verification (Research)

**Goal**: Confirm the actual file format expected by the web client

**Steps**:
1. Download a web-uploaded file from S3 using the HAR file's presigned URL (if still valid)
2. Examine the first 50 bytes in hex to confirm structure
3. Download a CLI-uploaded file and compare
4. Read the web decryption code thoroughly to understand the exact flow

**Files to examine**:
- `web/packages/base/crypto/libsodium.ts` - `decryptStreamBytes` function
- `web/packages/media/file.ts` - `decryptRemoteFile` function
- Server API responses to understand data flow

**Expected outcome**: Definitive answer on whether header should be prepended or not

### Phase 2: Implementation (Fix)

**If Hypothesis 1 is correct (header must be prepended):**

Modify `cli/pkg/uploader/encryptor.go`:

```go
// EncryptFile encrypts a file using ChaCha20-Poly1305 stream cipher
// Returns the nonce (header) and encrypted file size
func EncryptFile(inputPath, outputPath string, key []byte) (nonce []byte, size int64, err error) {
    // ... existing setup code ...

    // Create encryptor
    encryptor, header, err := crypto.NewEncryptor(key)
    if err != nil {
        return nil, 0, fmt.Errorf("failed to create encryptor: %w", err)
    }

    // WRITE HEADER FIRST (24 bytes)
    written, err := writer.Write(header)
    if err != nil {
        return nil, 0, fmt.Errorf("failed to write header: %w", err)
    }
    totalWritten := int64(written)

    // Then write encrypted chunks...
    // ... rest of existing encryption logic ...

    return header, totalWritten, nil
}
```

**Files to modify**:
- `cli/pkg/uploader/encryptor.go` - Lines 83-86 (remove comment and write header)

**Testing**:
1. Upload a test image via CLI
2. Verify it displays correctly in web interface
3. Verify thumbnail still works
4. Test with various file sizes (small, exactly 4MB, larger files)
5. Test multipart uploads for files > 20MB

**If Hypothesis 2 is correct (metadata issue):**

Debug and compare:
- CLI metadata format vs web metadata format
- Base64 encoding (standard vs URL-safe)
- Field names and structure

### Phase 3: Validation

**Test scenarios**:
1. Upload small image (< 1MB)
2. Upload 4MB image (exactly one buffer)
3. Upload 10MB image (multiple buffers, single part)
4. Upload 50MB image (multipart upload)
5. Verify all images display in web, mobile, and desktop clients
6. Verify thumbnails work
7. Verify EXIF data is preserved
8. Re-upload same file (deduplication test)

**Success criteria**:
- All uploaded images display correctly in web interface
- No "cipher is too short" errors
- Thumbnails continue to work
- File sizes match expected encrypted sizes (original + overhead)
- Deduplication still works

## Risk Assessment

### Low Risk
- Adding header to file increases size by only 24 bytes (negligible)
- Header is already generated and available
- Change is localized to one function
- Backward compatible (new uploads will work, old ones still have the issue)

### Potential Issues
1. **Existing CLI-uploaded files** will remain broken (can't fix retroactively without re-upload)
2. **Storage format change**: If we prepend header, the metadata's `decryptionHeader` field becomes redundant (though we should keep it for compatibility)
3. **Testing coverage**: Need to test with multiple file sizes and upload methods

## Alternative Solutions

### Alternative 1: Fix Web Client to Not Expect Header
**Pros**: CLI code is correct according to the architecture
**Cons**: Requires changes to web, mobile, desktop clients - much more invasive

### Alternative 2: Hybrid Approach
Store header in both places:
- Prepend to file (for direct decryption)
- Keep in metadata (for backward compatibility)

**Pros**: Maximum compatibility
**Cons**: Redundant data, slightly larger API payloads

## Files Involved

### Files to Modify
- `cli/pkg/uploader/encryptor.go` (primary fix)

### Files to Test
- `cli/cmd/upload.go` (upload command)
- `cli/pkg/uploader/uploader.go` (upload orchestration)
- `cli/pkg/uploader/s3_uploader.go` (S3 upload logic)

### Reference Files (no changes needed)
- `web/packages/base/crypto/libsodium.ts`
- `web/packages/gallery/services/upload/upload-service.ts`
- `cli/internal/crypto/stream.go`

## Debugging Strategy

### Step 1: Create a Minimal Reproduction

Upload a test file and capture ALL debug output:

```bash
cd cli/
go build -o cli.exe .
.\cli.exe upload test_image.jpg --album "Test Album" > upload_log.txt 2>&1
```

The debug output at `uploader.go:431` shows the exact JSON sent to the API. Capture:
1. `File.DecryptionHeader` value
2. `File.Size` value
3. `File.ObjectKey` value
4. `Thumbnail.DecryptionHeader` value (for comparison)
5. `Thumbnail.Size` value

### Step 2: Verify Encrypted File Size

Check that the encrypted file on disk matches the reported size:

```bash
# In the CLI code, add debug output before upload:
fileInfo, _ := os.Stat(encryptedFilePath)
fmt.Printf("Debug - Encrypted file on disk: %d bytes\n", fileInfo.Size())
```

Confirm this matches `data.EncryptedFileSize`.

### Step 3: Examine Encrypted File Structure

Add hex dump of first and last 100 bytes of the encrypted file:

```go
// After encrypting file, before upload:
data, _ := os.ReadFile(encryptedFilePath)
fmt.Printf("Debug - First 100 bytes (hex): %x\n", data[:min(100, len(data))])
fmt.Printf("Debug - Last 100 bytes (hex): %x\n", data[max(0, len(data)-100):])
fmt.Printf("Debug - Total size: %d bytes\n", len(data))
```

Look for:
- Does the file start with encrypted data (not a header)?
- Does the last chunk have the proper TAG_FINAL structure (17 bytes minimum)?
- Is the file length a multiple of the chunk size + overhead?

### Step 4: Compare with Web-Uploaded File

Upload the same image via the web interface, then use the HAR file or browser dev tools to:
1. Capture the API response for a web-uploaded file
2. Download the encrypted file from S3
3. Compare the structure with CLI-uploaded file

### Step 5: Test Decryption Locally in CLI

Add a test that:
1. Encrypts a small test file
2. Immediately decrypts it using the same crypto library
3. Verifies the roundtrip works

```go
// Test in encryptor_test.go:
func TestEncryptDecryptRoundtrip(t *testing.T) {
    // Create test file
    // Encrypt with EncryptFile
    // Decrypt using crypto.NewDecryptor
    // Verify content matches
}
```

If this fails, the bug is in the encryption logic. If it succeeds, the bug is in how the web client receives or processes the data.

## Next Steps

1. ✅ Complete Phase 1 (Verification) - **BLOCKED: Need to run the debugging strategy above**
2. ⏹️ Implement Phase 2 based on verification results
3. ⏹️ Execute Phase 3 (Testing) with comprehensive test cases
4. ⏹️ Document the fix and update CLAUDE.md if needed
5. ⏹️ Consider adding automated tests to prevent regression

## Open Questions to Resolve

1. ✅ Does the web client's `decryptStreamBytes` expect the header prepended? **ANSWERED: NO, header is from metadata**
2. ⏹️ What is the EXACT size of CLI-encrypted files vs the reported size?
3. ⏹️ Does the CLI produce valid stream cipher output that can be decrypted?
4. ⏹️ Is there a boundary or alignment issue with chunk sizes?
5. ⏹️ Are there any existing CLI-uploaded files in production that would need migration?
6. ⏹️ Do mobile/desktop clients handle decryption the same way as web?

## Summary for User

I've analyzed the code thoroughly but haven't pinpointed the exact bug yet. The CLI's encryption approach is theoretically correct - it stores the header separately in metadata, which matches what the web client expects.

**Most likely issue**: There's a subtle bug in the encryption loop that produces malformed chunks or incorrect sizes.

**Recommended action**: Run the debugging strategy above to capture exact sizes and hex dumps of the encrypted files. This will definitively show whether:
- The encrypted file size is correct
- The chunk structure is valid
- The decryption header is properly formatted

Once we have this data, the fix should be straightforward.
