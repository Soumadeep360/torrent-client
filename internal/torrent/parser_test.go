package torrent

import (
	"testing"
)

func TestSplitPieces(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantPieces  int
		expectError bool
	}{
		{
			name:        "valid 2 pieces",
			input:       "12345678901234567890abcdefghijklmnopqrst", // 40 bytes = 2 pieces
			wantPieces:  2,
			expectError: false,
		},
		{
			name:        "invalid length",
			input:       "123", // 3 bytes, not divisible by 20
			wantPieces:  0,
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			wantPieces:  0,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pieces, err := splitPieces(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(pieces) != tt.wantPieces {
				t.Errorf("got %d pieces, want %d", len(pieces), tt.wantPieces)
			}

			// Verify each piece is 20 bytes
			for i, piece := range pieces {
				if len(piece) != 20 {
					t.Errorf("piece %d has length %d, want 20", i, len(piece))
				}
			}
		})
	}
}

func TestTorrentMetaPieceSize(t *testing.T) {
	meta := &TorrentMeta{
		Length:      1000,
		PieceLength: 300,
		Pieces:      make([][]byte, 4), // 4 pieces
	}

	tests := []struct {
		index    int
		wantSize int
	}{
		{0, 300},  // First piece: full size
		{1, 300},  // Middle piece: full size
		{2, 300},  // Second to last: full size
		{3, 100},  // Last piece: remainder (1000 % 300 = 100)
		{-1, 0},   // Invalid: negative index
		{4, 0},    // Invalid: out of bounds
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			size := meta.PieceSize(tt.index)
			if size != tt.wantSize {
				t.Errorf("PieceSize(%d) = %d, want %d", tt.index, size, tt.wantSize)
			}
		})
	}
}

func TestNumPieces(t *testing.T) {
	meta := &TorrentMeta{
		Pieces: make([][]byte, 42),
	}

	if got := meta.NumPieces(); got != 42 {
		t.Errorf("NumPieces() = %d, want 42", got)
	}
}
