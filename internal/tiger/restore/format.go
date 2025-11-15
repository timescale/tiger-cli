package restore

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DumpFormat represents the format of a database dump file
type DumpFormat string

const (
	FormatPlain     DumpFormat = "plain"
	FormatPlainGzip DumpFormat = "plain.gz"
	FormatCustom    DumpFormat = "custom"
	FormatTar       DumpFormat = "tar"
	FormatDirectory DumpFormat = "directory"
	FormatUnknown   DumpFormat = "unknown"
)

// detectFileFormat detects the format of a dump file based on extension and content
func detectFileFormat(filePath string, format string) (DumpFormat, error) {
	// If format explicitly specified, use it
	if format != "" {
		switch strings.ToLower(format) {
		case "plain":
			return FormatPlain, nil
		case "custom":
			return FormatCustom, nil
		case "tar":
			return FormatTar, nil
		case "directory":
			return FormatDirectory, nil
		default:
			return FormatUnknown, fmt.Errorf("unsupported format: %s", format)
		}
	}

	// Handle stdin
	if filePath == "-" {
		// For stdin, assume plain SQL (can't detect format without reading)
		return FormatPlain, nil
	}

	// Check if it's a directory
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return FormatUnknown, fmt.Errorf("failed to stat file: %w", err)
	}

	if fileInfo.IsDir() {
		// Check if it's a pg_dump directory format
		if isDirectoryFormat(filePath) {
			return FormatDirectory, nil
		}
		return FormatUnknown, fmt.Errorf("directory is not a valid pg_dump directory format")
	}

	// Auto-detect based on extension
	ext := strings.ToLower(filepath.Ext(filePath))
	basename := strings.TrimSuffix(filepath.Base(filePath), ext)

	// Check for .sql.gz
	if ext == ".gz" && strings.HasSuffix(strings.ToLower(basename), ".sql") {
		return FormatPlainGzip, nil
	}

	// Check other extensions
	switch ext {
	case ".sql":
		return FormatPlain, nil
	case ".tar":
		return FormatTar, nil
	case ".dump", ".custom":
		return FormatCustom, nil
	}

	// Try to detect format by reading file header
	return detectFormatByContent(filePath)
}

// detectFormatByContent detects format by reading the file header
func detectFormatByContent(filePath string) (DumpFormat, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return FormatUnknown, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read first few bytes
	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && err != io.EOF {
		return FormatUnknown, fmt.Errorf("failed to read file header: %w", err)
	}
	header = header[:n]

	// Check for gzip magic number
	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		// It's gzipped - check if it's SQL
		gzReader, err := gzip.NewReader(bytes.NewReader(header))
		if err == nil {
			defer gzReader.Close()
			return FormatPlainGzip, nil
		}
		return FormatUnknown, fmt.Errorf("gzipped file but not valid gzip format")
	}

	// Check for pg_dump custom format magic string
	// Custom format starts with "PGDMP" (PostgreSQL Dump)
	if len(header) >= 5 && string(header[:5]) == "PGDMP" {
		return FormatCustom, nil
	}

	// Check for tar format
	// Tar files have a specific structure at offset 257
	if len(header) >= 262 && string(header[257:262]) == "ustar" {
		return FormatTar, nil
	}

	// Check if it looks like plain SQL (heuristic)
	if looksLikePlainSQL(header) {
		return FormatPlain, nil
	}

	return FormatUnknown, fmt.Errorf("unable to detect file format")
}

// isDirectoryFormat checks if a directory is a pg_dump directory format
func isDirectoryFormat(dirPath string) bool {
	// pg_dump directory format has a toc.dat file
	tocPath := filepath.Join(dirPath, "toc.dat")
	if _, err := os.Stat(tocPath); err == nil {
		return true
	}
	return false
}

// looksLikePlainSQL uses heuristics to check if content looks like SQL
func looksLikePlainSQL(content []byte) bool {
	// Convert to string for easier checking
	text := strings.ToLower(string(content))

	// Common SQL keywords at the beginning of dumps
	sqlKeywords := []string{
		"--", // SQL comment
		"/*", // SQL block comment
		"set ",
		"select ",
		"create ",
		"insert ",
		"drop ",
		"alter ",
		"begin",
	}

	// Check if any keyword appears near the start
	for _, keyword := range sqlKeywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}

	return false
}

// String returns the string representation of DumpFormat
func (f DumpFormat) String() string {
	return string(f)
}

// RequiresPgRestore returns true if the format requires pg_restore tool
func (f DumpFormat) RequiresPgRestore() bool {
	return f == FormatCustom || f == FormatTar || f == FormatDirectory
}
