package toolutil

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
)

// validateFileOrBase64 enforces the mutually-exclusive file_path /
// content_base64 contract shared by every upload-style tool input.
func validateFileOrBase64(op, filePath, contentBase64 string) error {
	hasFilePath := filePath != ""
	hasBase64 := contentBase64 != ""
	if hasFilePath && hasBase64 {
		return fmt.Errorf("%s: provide either file_path or content_base64, not both", op)
	}
	if !hasFilePath && !hasBase64 {
		return fmt.Errorf("%s: either file_path or content_base64 is required", op)
	}
	return nil
}

// OpenFileOrBase64Source resolves the mutually-exclusive file_path /
// content_base64 upload input pair into a streaming reader plus the source
// size and a cleanup func the caller must defer. file_path is opened and
// validated against the configured upload size limit and streamed without
// buffering; content_base64 is decoded in memory. All errors are prefixed
// with op, matching the per-tool error convention.
func OpenFileOrBase64Source(op, filePath, contentBase64 string) (reader io.Reader, size int64, cleanup func(), err error) {
	if invalidErr := validateFileOrBase64(op, filePath, contentBase64); invalidErr != nil {
		return nil, 0, nil, invalidErr
	}
	if filePath != "" {
		cfg := GetUploadConfig()
		f, info, openErr := OpenAndValidateFile(filePath, cfg.MaxFileSize)
		if openErr != nil {
			return nil, 0, nil, fmt.Errorf("%s: %w", op, openErr)
		}
		return f, info.Size(), func() { _ = f.Close() }, nil
	}
	decoded, decodeErr := base64.StdEncoding.DecodeString(contentBase64)
	if decodeErr != nil {
		return nil, 0, nil, fmt.Errorf("%s: invalid base64 content: %w", op, decodeErr)
	}
	return bytes.NewReader(decoded), int64(len(decoded)), func() {}, nil
}

// ReadFileOrBase64 resolves the mutually-exclusive file_path / content_base64
// upload input pair into an in-memory bytes.Reader, for endpoints that need a
// seekable or length-aware body. All errors are prefixed with op.
func ReadFileOrBase64(op, filePath, contentBase64 string) (*bytes.Reader, error) {
	if err := validateFileOrBase64(op, filePath, contentBase64); err != nil {
		return nil, err
	}
	if filePath != "" {
		cfg := GetUploadConfig()
		f, info, err := OpenAndValidateFile(filePath, cfg.MaxFileSize)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		defer func() { _ = f.Close() }()

		data := make([]byte, info.Size())
		if _, err = io.ReadFull(f, data); err != nil {
			return nil, fmt.Errorf("%s: reading file: %w", op, err)
		}
		return bytes.NewReader(data), nil
	}
	decoded, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid base64 content: %w", op, err)
	}
	return bytes.NewReader(decoded), nil
}
