package filewriter

import (
	"fmt"
	"os"
)

// Writer handles writing and reading pieces from the output file
type Writer struct {
	file        *os.File
	totalSize   int64
	pieceLength int // standard piece length for offset calculation
}

// NewWriter creates (or truncates) the output file and pre-allocates its full size
func NewWriter(filePath string, totalSize int64, pieceLength int) (*Writer, error) {
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	if err := file.Truncate(totalSize); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to allocate file space: %w", err)
	}
	return &Writer{file: file, totalSize: totalSize, pieceLength: pieceLength}, nil
}

// NewWriterForResume opens the file for resume if it already exists with the correct
// size, or falls back to NewWriter (create + pre-allocate) if it doesn't.
func NewWriterForResume(filePath string, totalSize int64, pieceLength int) (*Writer, error) {
	fi, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return NewWriter(filePath, totalSize, pieceLength)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	file, err := os.OpenFile(filePath, os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for resume: %w", err)
	}
	if fi.Size() != totalSize {
		if err := file.Truncate(totalSize); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to resize file to %d: %w", totalSize, err)
		}
	}
	return &Writer{file: file, totalSize: totalSize, pieceLength: pieceLength}, nil
}

// ReadAt reads length bytes at the given offset (used by the resume/idempotency check)
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

// WritePiece writes data to the correct file offset for pieceIndex.
// Uses the standard pieceLength for offset calculation — the last piece is
// smaller but still lives at index * pieceLength in the file.
// WriteAt is safe for concurrent calls on non-overlapping regions.
func (w *Writer) WritePiece(pieceIndex int, data []byte) error {
	offset := int64(pieceIndex) * int64(w.pieceLength)
	n, err := w.file.WriteAt(data, offset)
	if err != nil {
		return fmt.Errorf("failed to write piece %d: %w", pieceIndex, err)
	}
	if n != len(data) {
		return fmt.Errorf("incomplete write: wrote %d/%d bytes", n, len(data))
	}
	return nil
}

// Close syncs all writes to disk and closes the file
func (w *Writer) Close() error {
	if err := w.file.Sync(); err != nil {
		w.file.Close()
		return fmt.Errorf("failed to sync file: %w", err)
	}
	return w.file.Close()
}

// VerifyFileSize checks the file on disk matches the expected total size
func (w *Writer) VerifyFileSize() error {
	fi, err := w.file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	if fi.Size() != w.totalSize {
		return fmt.Errorf("file size mismatch: expected %d, got %d", w.totalSize, fi.Size())
	}
	return nil
}
