package uploader

import (
	"bytes"
	"github.com/ente-io/cli/internal/crypto"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestEncryptDecryptRoundtrip tests that encrypted files can be decrypted successfully
func TestEncryptDecryptRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{"Small file (100 bytes)", 100},
		{"Medium file (1 MB)", 1 * 1024 * 1024},
		{"Exactly 4 MB", 4 * 1024 * 1024},
		{"Large file (8 MB)", 8 * 1024 * 1024},
		{"Just over 4 MB", 4*1024*1024 + 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tempDir, err := os.MkdirTemp("", "encrypt-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Create test file with known content
			originalPath := filepath.Join(tempDir, "original.dat")
			originalData := make([]byte, tt.size)
			for i := range originalData {
				originalData[i] = byte(i % 256)
			}
			if err := os.WriteFile(originalPath, originalData, 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Generate encryption key
			key, err := GenerateFileKey()
			if err != nil {
				t.Fatalf("Failed to generate key: %v", err)
			}

			// Encrypt file
			encryptedPath := filepath.Join(tempDir, "encrypted.dat")
			header, encryptedSize, err := EncryptFile(originalPath, encryptedPath, key)
			if err != nil {
				t.Fatalf("Failed to encrypt file: %v", err)
			}

			t.Logf("Original size: %d, Encrypted size: %d, Header size: %d",
				tt.size, encryptedSize, len(header))

			// Verify encrypted file exists and has correct size
			encryptedInfo, err := os.Stat(encryptedPath)
			if err != nil {
				t.Fatalf("Encrypted file not found: %v", err)
			}
			if encryptedInfo.Size() != encryptedSize {
				t.Errorf("Encrypted file size mismatch: got %d, reported %d",
					encryptedInfo.Size(), encryptedSize)
			}

			// Decrypt file
			decryptedData, err := decryptFile(encryptedPath, header, key)
			if err != nil {
				t.Fatalf("Failed to decrypt file: %v", err)
			}

			// Verify decrypted data matches original
			if len(decryptedData) != len(originalData) {
				t.Errorf("Decrypted data size mismatch: got %d, want %d",
					len(decryptedData), len(originalData))
			}

			if !bytes.Equal(decryptedData, originalData) {
				t.Errorf("Decrypted data does not match original")
				// Show first difference
				for i := 0; i < len(originalData) && i < len(decryptedData); i++ {
					if originalData[i] != decryptedData[i] {
						t.Logf("First difference at byte %d: got %02x, want %02x",
							i, decryptedData[i], originalData[i])
						break
					}
				}
			}
		})
	}
}

// decryptFile decrypts a file encrypted with EncryptFile
func decryptFile(encryptedPath string, header []byte, key []byte) ([]byte, error) {
	// Read encrypted file
	encryptedFile, err := os.Open(encryptedPath)
	if err != nil {
		return nil, err
	}
	defer encryptedFile.Close()

	// Create decryptor with header
	decryptor, err := crypto.NewDecryptor(key, header)
	if err != nil {
		return nil, err
	}

	// Read and decrypt in chunks
	var decryptedChunks [][]byte
	buf := make([]byte, encryptionBufferSize+crypto.XChaCha20Poly1305IetfABYTES)

	for {
		n, err := encryptedFile.Read(buf)
		if err != nil && err != io.EOF {
			return nil, err
		}
		if n == 0 {
			break
		}

		// Decrypt chunk
		decrypted, tag, err := decryptor.Pull(buf[:n])
		if err != nil {
			return nil, err
		}

		decryptedChunks = append(decryptedChunks, decrypted)

		// Check if this was the final chunk
		if tag == byte(crypto.TagFinal) {
			break
		}
	}

	// Concatenate all decrypted chunks
	var result []byte
	for _, chunk := range decryptedChunks {
		result = append(result, chunk...)
	}

	return result, nil
}

// TestEncryptDataRoundtrip tests the simple EncryptData function
func TestEncryptDataRoundtrip(t *testing.T) {
	data := []byte("Hello, World! This is a test of the encryption system.")

	// Generate key
	key, err := GenerateFileKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Encrypt
	encrypted, nonce, err := EncryptData(data, key)
	if err != nil {
		t.Fatalf("Failed to encrypt data: %v", err)
	}

	t.Logf("Original: %d bytes, Encrypted: %d bytes, Nonce: %d bytes",
		len(data), len(encrypted), len(nonce))

	// Decrypt using the same method as web client would
	decryptor, err := crypto.NewDecryptor(key, nonce)
	if err != nil {
		t.Fatalf("Failed to create decryptor: %v", err)
	}

	decrypted, tag, err := decryptor.Pull(encrypted)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if tag != byte(crypto.TagFinal) {
		t.Errorf("Expected TagFinal (%d), got %d", crypto.TagFinal, tag)
	}

	if !bytes.Equal(decrypted, data) {
		t.Errorf("Decrypted data does not match original")
		t.Logf("Original: %s", data)
		t.Logf("Decrypted: %s", decrypted)
	}
}
