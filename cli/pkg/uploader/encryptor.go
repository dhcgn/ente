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

	// Write header to output
	if _, err := writer.Write(header); err != nil {
		return nil, 0, fmt.Errorf("failed to write header: %w", err)
	}
	totalWritten := int64(len(header))

	// Read and encrypt in chunks
	buf := make([]byte, encryptionBufferSize)
	for {
		n, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			return nil, 0, fmt.Errorf("failed to read from input file: %w", err)
		}
		if n == 0 {
			break
		}

		// Determine tag (final chunk or message)
		tag := byte(crypto.TagMessage)
		if err == io.EOF {
			tag = byte(crypto.TagFinal)
		}

		// Encrypt chunk
		encrypted, encErr := encryptor.Push(buf[:n], tag)
		if encErr != nil {
			return nil, 0, fmt.Errorf("failed to encrypt chunk: %w", encErr)
		}

		// Write encrypted chunk
		written, writeErr := writer.Write(encrypted)
		if writeErr != nil {
			return nil, 0, fmt.Errorf("failed to write encrypted chunk: %w", writeErr)
		}
		totalWritten += int64(written)

		if err == io.EOF {
			break
		}
	}

	if err := writer.Flush(); err != nil {
		return nil, 0, fmt.Errorf("failed to flush output: %w", err)
	}

	return header, totalWritten, nil
}

// EncryptData encrypts data using ChaCha20-Poly1305
// Returns encrypted data and nonce
func EncryptData(data []byte, key []byte) (encrypted, nonce []byte, err error) {
	encrypted, nonce, err = crypto.EncryptChaCha20poly1305(data, key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encrypt data: %w", err)
	}
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
