package filewriter

import (
	"fmt"
	"os"
	"sync"
)

// Writer handles writing pieces to disk concurrently
type Writer struct {
	file       *os.File
	filePath   string
	totalSize  int64
	mu         sync.Mutex
	writeCount int
}

// NewWriter creates a new file writer (creates or truncates the file)
func NewWriter(filePath string, totalSize int64) (*Writer, error) {
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	if err := file.Truncate(totalSize); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to allocate file space: %w", err)
	}

	return &Writer{
		file:      file,
		filePath:  filePath,
		totalSize: totalSize,
	}, nil
}

// NewWriterForResume opens the file for resume: if it exists and has the expected size,
// it is opened for read/write without truncation so existing pieces can be verified and
// only missing ones downloaded. If the file does not exist or has wrong size, it is
// created or truncated to totalSize.
func NewWriterForResume(filePath string, totalSize int64) (*Writer, error) {
	fi, err := os.Stat(filePath)
	if err == nil {
		// File exists
		file, openErr := os.OpenFile(filePath, os.O_RDWR, 0666)
		if openErr != nil {
			return nil, fmt.Errorf("failed to open file for resume: %w", openErr)
		}
		if fi.Size() != totalSize {
			if truncErr := file.Truncate(totalSize); truncErr != nil {
				file.Close()
				return nil, fmt.Errorf("failed to resize file to %d: %w", totalSize, truncErr)
			}
		}
		return &Writer{
			file:      file,
			filePath:  filePath,
			totalSize: totalSize,
		}, nil
	}
	if os.IsNotExist(err) {
		return NewWriter(filePath, totalSize)
	}
	return nil, fmt.Errorf("failed to stat file: %w", err)
}

// ReadAt reads length bytes at offset from the file (for verifying existing pieces)
func (w *Writer) ReadAt(offset int64, length int) ([]byte, error) {
	buf := make([]byte, length)
	n, err := w.file.ReadAt(buf, offset)
	if err != nil {
		return nil, fmt.Errorf("read at %d: %w", offset, err)
	}
	if n != length {
		return nil, fmt.Errorf("read at %d: got %d bytes, want %d", offset, n, length)
	}
	return buf, nil
}

// WritePiece writes a piece to the file at the specified offset
// This method is safe for concurrent use from multiple goroutines
func (w *Writer) WritePiece(pieceIndex int, pieceLength int, data []byte) error {
	if len(data) != pieceLength {
		return fmt.Errorf("data length mismatch: expected %d, got %d", pieceLength, len(data))
	}

	// Calculate file offset for this piece
	offset := int64(pieceIndex) * int64(pieceLength)

	// WriteAt is thread-safe and can be called concurrently
	// It writes at the specified offset without changing the file position
	n, err := w.file.WriteAt(data, offset)
	if err != nil {
		return fmt.Errorf("failed to write piece %d: %w", pieceIndex, err)
	}

	if n != len(data) {
		return fmt.Errorf("incomplete write: wrote %d/%d bytes", n, len(data))
	}

	// Track write count (optional, for monitoring)
	w.mu.Lock()
	w.writeCount++
	w.mu.Unlock()

	return nil
}

// Close closes the file and ensures all writes are flushed
func (w *Writer) Close() error {
	// Sync to ensure all data is written to disk
	if err := w.file.Sync(); err != nil {
		w.file.Close()
		return fmt.Errorf("failed to sync file: %w", err)
	}

	// Close the file
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	return nil
}

// GetWriteCount returns the number of pieces written (thread-safe)
func (w *Writer) GetWriteCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writeCount
}

// GetFilePath returns the path of the output file
func (w *Writer) GetFilePath() string {
	return w.filePath
}

// WriteAllPieces writes all pieces to the file
// This is a convenience method for writing pieces sequentially
func WriteAllPieces(filePath string, pieces [][]byte, pieceLength int) error {
	// Calculate total size
	totalSize := int64(0)
	for _, piece := range pieces {
		totalSize += int64(len(piece))
	}

	// Create writer
	writer, err := NewWriter(filePath, totalSize)
	if err != nil {
		return err
	}
	defer writer.Close()

	// Write each piece
	for i, pieceData := range pieces {
		if err := writer.WritePiece(i, pieceLength, pieceData); err != nil {
			return fmt.Errorf("failed to write piece %d: %w", i, err)
		}
	}

	return nil
}

// VerifyFileSize checks if the file size matches expected size
func (w *Writer) VerifyFileSize() error {
	fileInfo, err := w.file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	if fileInfo.Size() != w.totalSize {
		return fmt.Errorf("file size mismatch: expected %d, got %d", w.totalSize, fileInfo.Size())
	}

	return nil
}
