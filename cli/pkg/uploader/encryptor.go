package uploader

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/ente-io/cli/internal/crypto"
	"github.com/ente-io/cli/utils/encoding"
	"golang.org/x/crypto/nacl/secretbox"
	"io"
	"os"
)

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

const (
	encryptionBufferSize = 4 * 1024 * 1024 // 4MB chunks
	keySize              = 32               // ChaCha20-Poly1305 key size
	nonceSize            = 24               // SecretBox nonce size
)

// GenerateFileKey generates a random 32-byte key for file encryption
func GenerateFileKey() ([]byte, error) {
	key := make([]byte, keySize)
	_, err := rand.Read(key)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
}

// EncryptFileKeyWithCollectionKey encrypts the file key using the collection key via SecretBox
func EncryptFileKeyWithCollectionKey(fileKey, collectionKey []byte) (encryptedKey, nonce string, err error) {
	if len(collectionKey) != keySize {
		return "", "", fmt.Errorf("invalid collection key size: %d (expected %d)", len(collectionKey), keySize)
	}

	// Generate random nonce
	nonceBytes := make([]byte, nonceSize)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Convert to fixed-size arrays for secretbox
	var key [32]byte
	var n [24]byte
	copy(key[:], collectionKey)
	copy(n[:], nonceBytes)

	// Encrypt using SecretBox
	encrypted := secretbox.Seal(nil, fileKey, &n, &key)

	// Return base64-encoded strings
	return base64.StdEncoding.EncodeToString(encrypted),
		base64.StdEncoding.EncodeToString(nonceBytes),
		nil
}

// EncryptFile encrypts a file using ChaCha20-Poly1305 stream cipher
// Returns the nonce (header) and encrypted file size
func EncryptFile(inputPath, outputPath string, key []byte) (nonce []byte, size int64, err error) {
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open input file: %w", err)
	}
	defer inputFile.Close()

	outputFile, err := os.Create(outputPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	reader := bufio.NewReader(inputFile)
	writer := bufio.NewWriter(outputFile)
	defer writer.Flush()

	// Create encryptor
	encryptor, header, err := crypto.NewEncryptor(key)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create encryptor: %w", err)
	}

	// NOTE: Do NOT write header to output file
	// The header must be stored separately in metadata (decryptionHeader field)
	// Only encrypted chunks are written to the file uploaded to S3
	totalWritten := int64(0)

	// Read and encrypt in chunks
	buf := make([]byte, encryptionBufferSize)
	chunkCount := 0
	needsFinalChunk := false

	fmt.Printf("\nDebug [EncryptFile] - Starting encryption\n")
	fmt.Printf("Debug [EncryptFile] - Header (nonce) size: %d bytes\n", len(header))
	fmt.Printf("Debug [EncryptFile] - Header (hex): %x\n", header)

	for {
		n, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			return nil, 0, fmt.Errorf("failed to read from input file: %w", err)
		}

		// If we read zero bytes, we're done
		if n == 0 {
			break
		}

		chunkCount++

		// Determine if this is the last chunk:
		// 1. If we got EOF, this is definitely the last data
		// 2. If we read less than a full buffer, this must be the last data
		//    (there's no more data to fill the buffer)
		isLastChunk := (err == io.EOF) || (n < encryptionBufferSize)

		fmt.Printf("Debug [EncryptFile] - Chunk %d: read %d bytes, err=%v, isLast=%v\n",
			chunkCount, n, err, isLastChunk)

		// Determine tag based on whether this is the last chunk
		tag := byte(crypto.TagMessage)
		if isLastChunk {
			tag = byte(crypto.TagFinal)
			needsFinalChunk = false // No need for empty final chunk
		} else {
			// If this is a full buffer, we might need an empty final chunk later
			needsFinalChunk = true
		}

		// Encrypt chunk (can be empty for final chunk)
		encrypted, encErr := encryptor.Push(buf[:n], tag)
		if encErr != nil {
			return nil, 0, fmt.Errorf("failed to encrypt chunk: %w", encErr)
		}

		fmt.Printf("Debug [EncryptFile] - Chunk %d: plaintext=%d bytes, encrypted=%d bytes, tag=%d\n",
			chunkCount, n, len(encrypted), tag)

		// Write encrypted chunk
		written, writeErr := writer.Write(encrypted)
		if writeErr != nil {
			return nil, 0, fmt.Errorf("failed to write encrypted chunk: %w", writeErr)
		}
		totalWritten += int64(written)
	}

	// If the file size was an exact multiple of the buffer size, we need to write
	// an empty final chunk with TagFinal
	if needsFinalChunk {
		chunkCount++
		fmt.Printf("Debug [EncryptFile] - Chunk %d: writing empty final chunk\n", chunkCount)

		encrypted, encErr := encryptor.Push([]byte{}, byte(crypto.TagFinal))
		if encErr != nil {
			return nil, 0, fmt.Errorf("failed to encrypt final chunk: %w", encErr)
		}

		fmt.Printf("Debug [EncryptFile] - Chunk %d: plaintext=0 bytes, encrypted=%d bytes, tag=%d\n",
			chunkCount, len(encrypted), crypto.TagFinal)

		written, writeErr := writer.Write(encrypted)
		if writeErr != nil {
			return nil, 0, fmt.Errorf("failed to write final chunk: %w", writeErr)
		}
		totalWritten += int64(written)
	}

	if err := writer.Flush(); err != nil {
		return nil, 0, fmt.Errorf("failed to flush output: %w", err)
	}

	fmt.Printf("Debug [EncryptFile] - Encryption complete: %d chunks, %d total bytes\n",
		chunkCount, totalWritten)

	// Read back and show hex dump of first and last bytes
	if debugFile, err := os.Open(outputPath); err == nil {
		defer debugFile.Close()

		// First 100 bytes
		firstBytes := make([]byte, 100)
		if n, _ := debugFile.Read(firstBytes); n > 0 {
			fmt.Printf("Debug [EncryptFile] - First %d bytes (hex): %x\n", n, firstBytes[:n])
		}

		// Last 100 bytes
		if fileInfo, err := debugFile.Stat(); err == nil {
			fileSize := fileInfo.Size()
			fmt.Printf("Debug [EncryptFile] - File size on disk: %d bytes\n", fileSize)

			if fileSize > 100 {
				lastBytes := make([]byte, 100)
				if _, err := debugFile.Seek(-100, io.SeekEnd); err == nil {
					if n, _ := debugFile.Read(lastBytes); n > 0 {
						fmt.Printf("Debug [EncryptFile] - Last %d bytes (hex): %x\n", n, lastBytes[:n])
					}
				}
			}
		}
	}

	return header, totalWritten, nil
}

// EncryptData encrypts data using ChaCha20-Poly1305
// Returns encrypted data and nonce
func EncryptData(data []byte, key []byte) (encrypted, nonce []byte, err error) {
	fmt.Printf("\nDebug [EncryptData] - Encrypting %d bytes\n", len(data))
	encrypted, nonce, err = crypto.EncryptChaCha20poly1305(data, key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encrypt data: %w", err)
	}
	fmt.Printf("Debug [EncryptData] - Result: %d encrypted bytes, nonce size: %d\n",
		len(encrypted), len(nonce))
	fmt.Printf("Debug [EncryptData] - Encrypted first 50 bytes (hex): %x\n",
		encrypted[:min(50, len(encrypted))])
	return encrypted, nonce, nil
}

// EncryptDataBase64 encrypts data and returns base64-encoded strings
func EncryptDataBase64(data []byte, key []byte) (encrypted, nonce string, err error) {
	encryptedBytes, nonceBytes, err := EncryptData(data, key)
	if err != nil {
		return "", "", err
	}
	return encoding.EncodeBase64(encryptedBytes), encoding.EncodeBase64(nonceBytes), nil
}
