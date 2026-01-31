package uploader

import (
	"fmt"
	"sync"
	"time"
)

// ProgressTracker tracks upload progress across multiple files
type ProgressTracker struct {
	totalFiles     int
	completedFiles int
	failedFiles    int
	skippedFiles   int
	totalBytes     int64
	uploadedBytes  int64
	currentFile    string
	startTime      time.Time
	mu             sync.RWMutex
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(totalFiles int, totalBytes int64) *ProgressTracker {
	return &ProgressTracker{
		totalFiles: totalFiles,
		totalBytes: totalBytes,
		startTime:  time.Now(),
	}
}

// SetCurrentFile sets the currently uploading file
func (p *ProgressTracker) SetCurrentFile(filename string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentFile = filename
}

// AddCompletedFile increments the completed file counter
func (p *ProgressTracker) AddCompletedFile() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.completedFiles++
}

// AddFailedFile increments the failed file counter
func (p *ProgressTracker) AddFailedFile() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failedFiles++
}

// AddSkippedFile increments the skipped file counter (duplicates)
func (p *ProgressTracker) AddSkippedFile() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.skippedFiles++
}

// AddUploadedBytes adds to the uploaded bytes counter
func (p *ProgressTracker) AddUploadedBytes(bytes int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.uploadedBytes += bytes
}

// GetStats returns current progress statistics
func (p *ProgressTracker) GetStats() (completed, failed, skipped, total int, uploadedBytes, totalBytes int64, currentFile string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.completedFiles, p.failedFiles, p.skippedFiles, p.totalFiles,
		p.uploadedBytes, p.totalBytes, p.currentFile
}

// Render returns a formatted progress string
func (p *ProgressTracker) Render() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	elapsed := time.Since(p.startTime)
	processed := p.completedFiles + p.failedFiles + p.skippedFiles

	// Calculate percentage
	var percentComplete float64
	if p.totalBytes > 0 {
		percentComplete = float64(p.uploadedBytes) / float64(p.totalBytes) * 100
	} else if p.totalFiles > 0 {
		percentComplete = float64(processed) / float64(p.totalFiles) * 100
	}

	// Format bytes
	uploadedStr := formatBytes(p.uploadedBytes)
	totalStr := formatBytes(p.totalBytes)

	// Calculate speed
	var speed string
	if elapsed.Seconds() > 0 {
		bytesPerSec := float64(p.uploadedBytes) / elapsed.Seconds()
		speed = fmt.Sprintf(" @ %s/s", formatBytes(int64(bytesPerSec)))
	}

	// Build progress bar
	barWidth := 30
	filled := int(percentComplete * float64(barWidth) / 100)
	if filled > barWidth {
		filled = barWidth
	}
	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}

	status := fmt.Sprintf("[%d/%d] %s [%s] %.1f%% (%s / %s)%s",
		processed, p.totalFiles,
		truncateFilename(p.currentFile, 30),
		bar,
		percentComplete,
		uploadedStr, totalStr,
		speed)

	if p.failedFiles > 0 {
		status += fmt.Sprintf(" | %d failed", p.failedFiles)
	}
	if p.skippedFiles > 0 {
		status += fmt.Sprintf(" | %d skipped", p.skippedFiles)
	}

	return status
}

// GetSummary returns a summary of the upload session
func (p *ProgressTracker) GetSummary() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	elapsed := time.Since(p.startTime)
	uploadedStr := formatBytes(p.uploadedBytes)

	summary := fmt.Sprintf("\nUpload complete in %s\n", elapsed.Round(time.Second))
	summary += fmt.Sprintf("  Completed: %d\n", p.completedFiles)
	if p.skippedFiles > 0 {
		summary += fmt.Sprintf("  Skipped (duplicates): %d\n", p.skippedFiles)
	}
	if p.failedFiles > 0 {
		summary += fmt.Sprintf("  Failed: %d\n", p.failedFiles)
	}
	summary += fmt.Sprintf("  Total uploaded: %s\n", uploadedStr)

	return summary
}

// formatBytes formats bytes into human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// truncateFilename truncates a filename to the specified length
func truncateFilename(filename string, maxLen int) string {
	if len(filename) <= maxLen {
		return filename
	}
	if maxLen <= 3 {
		return filename[:maxLen]
	}
	return filename[:maxLen-3] + "..."
}
