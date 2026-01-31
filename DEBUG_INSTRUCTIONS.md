# Debugging Instructions for "Cipher is Too Short" Bug

## What I've Done

I've added comprehensive debugging output to the CLI upload code to help identify the root cause of the "TypeError: cipher is too short" error that occurs when trying to view CLI-uploaded images in the web interface.

### Files Modified

1. **cli/pkg/uploader/encryptor.go**
   - Added debug output showing encryption process details
   - Tracks each chunk being encrypted (size, tag, etc.)
   - Shows hex dump of first and last bytes of encrypted file
   - Added debug output to `EncryptData` function (used for thumbnails)

2. **cli/pkg/uploader/uploader.go**
   - Added debug output showing encrypted data summary before upload
   - Shows decryption headers and sizes for both file and thumbnail

3. **cli/pkg/uploader/encryptor_test.go** (NEW)
   - Created comprehensive roundtrip tests
   - Tests encryption/decryption with various file sizes
   - Helps identify if the bug is in encryption logic itself

## How to Run the Diagnostic Tests

### Step 1: Run Unit Tests

```bash
cd cli/pkg/uploader
go test -v -run TestEncryptDecryptRoundtrip
go test -v -run TestEncryptDataRoundtrip
```

**Expected outcome:**
- If tests PASS: The encryption/decryption logic is correct
- If tests FAIL: There's a bug in the CLI's crypto implementation

### Step 2: Upload a Test File with Debug Output

```bash
cd cli/
go build -o cli.exe .

# Upload a small test image
.\cli.exe upload path\to\test_image.jpg --album "Debug Test" > upload_debug.log 2>&1
```

This will create `upload_debug.log` with detailed information about:
- Each encryption chunk (plaintext size, encrypted size, tag)
- Header (nonce) in hex
- First and last bytes of encrypted file in hex
- Final encrypted file size vs size on disk
- The exact JSON being sent to the API

### Step 3: Analyze the Debug Output

Look for these issues in `upload_debug.log`:

1. **Size Mismatch**
   ```
   Debug [EncryptFile] - File size on disk: XXXX bytes
   Debug [Upload] - Encrypted file size: YYYY bytes
   ```
   These should match! If not, there's a bug in size calculation.

2. **Missing Final Chunk**
   ```
   Debug [EncryptFile] - Chunk N: ... tag=2
   ```
   The last chunk should have `tag=2` (TagFinal). If not, the file is incomplete.

3. **Empty Chunks**
   Look for any chunks with `plaintext=0 bytes` that aren't the final chunk.

4. **Decryption Header Format**
   ```
   File decryption header: <base64 string> (len=32)
   ```
   The base64 string should decode to 24 bytes (the nonce).

### Step 4: Compare with Thumbnail

The debug output will show both file and thumbnail encryption. Compare:
- Do they use the same encryption process?
- Are the sizes calculated correctly for both?
- Do both have valid decryption headers?

Since thumbnails work but files don't, the difference will be illuminating.

## Common Issues to Look For

### Issue 1: Incorrect File Size

**Symptom**: `File size on disk` doesn't match `Encrypted file size`

**Cause**: The `totalWritten` calculation in `EncryptFile` is wrong

**Fix**: Verify all encrypted chunks are being written and counted correctly

### Issue 2: Missing Final Tag

**Symptom**: Last chunk doesn't have `tag=2` (TagFinal)

**Cause**: The loop exits before encrypting the final chunk

**Fix**: Review the loop logic in `encryptor.go:92-131`

### Issue 3: Extra Empty Chunk

**Symptom**: Files that are exact multiples of 4MB have an extra 17-byte chunk at the end

**Cause**: This is actually CORRECT behavior for stream cipher, but might cause issues if web client doesn't expect it

**Fix**: Verify web client can handle empty final chunks

### Issue 4: Incompatible Crypto Implementation

**Symptom**: Unit tests FAIL - decrypted data doesn't match original

**Cause**: The custom Go implementation in `cli/internal/crypto/stream.go` has bugs or is incompatible with libsodium

**Fix**: Consider using a proper libsodium binding instead of the custom implementation

## Next Steps After Diagnosis

Once you've run the tests and captured the debug output:

1. **If unit tests fail**: The bug is in the CLI's crypto implementation. Focus on `cli/internal/crypto/stream.go` or consider switching to a libsodium binding.

2. **If unit tests pass but uploads fail**: The bug is in how data is transmitted or how the web client receives it. Check:
   - Is the decryption header properly base64-encoded?
   - Does the API correctly store and return the header?
   - Does the web client download the complete file?

3. **If you find a size mismatch**: Fix the size calculation in `EncryptFile`.

4. **If you see structural issues**: Compare the hex dumps with a web-uploaded file to see the format difference.

## Comparing with Web-Uploaded Files

To compare CLI-uploaded vs web-uploaded files:

1. Upload the same image via web interface
2. Use browser dev tools (Network tab) to capture the file download
3. Look at the response to see:
   - The `decryptionHeader` value
   - The `size` value
   - The actual bytes downloaded from S3
4. Compare with the CLI debug output

Look for:
- Same header size (should be 24 bytes when decoded from base64)
- Same size format (encrypted data only, no header prepended)
- Same chunk structure (multiple chunks with overhead)

## If You Need Help

If you can't identify the issue after running these diagnostics, share:
1. The `upload_debug.log` file
2. Unit test results
3. File size being uploaded (original vs encrypted)
4. Whether thumbnails are encrypting successfully

With this information, the exact bug should become clear.
