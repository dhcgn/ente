package uploader

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"github.com/ente-io/cli/internal/api"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const (
	multipartPartSize = 20 * 1024 * 1024 // 20MB per part
)

// ComputeFileMD5 computes the MD5 hash of a file and returns it as base64
func ComputeFileMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to compute MD5: %w", err)
	}

	return base64.StdEncoding.EncodeToString(hash.Sum(nil)), nil
}

// ComputePartMD5s splits a file into parts and computes MD5 for each part
func ComputePartMD5s(filePath string, partSize int64) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	fileSize := fileInfo.Size()

	partCount := (fileSize + partSize - 1) / partSize
	md5s := make([]string, partCount)

	buf := make([]byte, partSize)
	for i := int64(0); i < partCount; i++ {
		// Read part
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read part %d: %w", i, err)
		}

		// Compute MD5
		hash := md5.Sum(buf[:n])
		md5s[i] = base64.StdEncoding.EncodeToString(hash[:])
	}

	return md5s, nil
}

// UploadToS3 uploads a file to S3 using a presigned URL
func UploadToS3(url string, filePath string, md5Hash string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	fmt.Printf("Debug - Uploading to S3: file=%s, size=%d bytes\n", filepath.Base(filePath), fileInfo.Size())

	req, err := http.NewRequest(http.MethodPut, url, file)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-MD5", md5Hash)
	req.ContentLength = fileInfo.Size()

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("Debug - S3 upload response: status=%d\n", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("S3 upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// MultipartPart represents a completed multipart upload part
type MultipartPart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

// CompleteMultipartUpload represents the XML payload for completing multipart upload
type CompleteMultipartUpload struct {
	XMLName xml.Name          `xml:"CompleteMultipartUpload"`
	Parts   []MultipartPart   `xml:"Part"`
}

// UploadMultipart uploads a large file using multipart upload
func UploadMultipart(urls *api.MultipartUploadURLs, filePath string, partMD5s []string, progress *ProgressTracker) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	etags := make([]string, len(urls.PartURLs))
	buf := make([]byte, multipartPartSize)

	// Upload each part
	for i, partURL := range urls.PartURLs {
		// Read part from file
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read part %d: %w", i, err)
		}
		if n == 0 {
			break
		}

		// Create request
		req, err := http.NewRequest(http.MethodPut, partURL, bytes.NewReader(buf[:n]))
		if err != nil {
			return fmt.Errorf("failed to create request for part %d: %w", i, err)
		}

		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Content-MD5", partMD5s[i])
		req.ContentLength = int64(n)

		// Upload part
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to upload part %d: %w", i, err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("part %d upload failed with status %d: %s", i, resp.StatusCode, string(body))
		}

		// Store ETag
		etag := resp.Header.Get("ETag")
		if etag == "" {
			resp.Body.Close()
			return fmt.Errorf("part %d upload succeeded but ETag is empty", i)
		}
		etags[i] = etag
		resp.Body.Close()

		// Update progress
		if progress != nil {
			progress.AddUploadedBytes(int64(n))
		}
	}

	// Complete multipart upload
	parts := make([]MultipartPart, len(etags))
	for i, etag := range etags {
		parts[i] = MultipartPart{
			PartNumber: i + 1,
			ETag:       etag,
		}
	}

	completePayload := CompleteMultipartUpload{Parts: parts}
	xmlData, err := xml.Marshal(completePayload)
	if err != nil {
		return fmt.Errorf("failed to marshal complete payload: %w", err)
	}

	// Send complete request
	req, err := http.NewRequest(http.MethodPost, urls.CompleteURL, bytes.NewReader(xmlData))
	if err != nil {
		return fmt.Errorf("failed to create complete request: %w", err)
	}
	req.Header.Set("Content-Type", "application/xml")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("complete multipart upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
