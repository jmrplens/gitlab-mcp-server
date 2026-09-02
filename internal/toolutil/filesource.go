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
		// The cap is enforced again while reading, not just against the size
		// os.Stat reported. A procfs entry is a regular file whose reported
		// size is zero and whose content is not, so a check that trusted
		// os.Stat streamed /proc/self/environ — this process's own
		// credentials — past a limit it had already satisfied.
		return newLimitedFileReader(op, f, cfg.MaxFileSize), info.Size(), func() { _ = f.Close() }, nil
	}
	decoded, decodeErr := decodeBase64Content(op, contentBase64)
	if decodeErr != nil {
		return nil, 0, nil, decodeErr
	}
	return bytes.NewReader(decoded), int64(len(decoded)), func() { /* in-memory reader; nothing to release */ }, nil
}

// limitedFileReader stops a streaming upload at the configured maximum and
// reports the refusal as a read error, so a source whose length could not be
// known in advance cannot exceed the limit merely by lying about its size.
type limitedFileReader struct {
	op      string
	inner   io.Reader
	read    int64
	maxSize int64
}

// newLimitedFileReader wraps r so it yields at most maxSize bytes. A maxSize
// of zero or less means unlimited, matching [OpenAndValidateFile].
func newLimitedFileReader(op string, r io.Reader, maxSize int64) io.Reader {
	if maxSize <= 0 {
		return r
	}
	return &limitedFileReader{op: op, inner: r, maxSize: maxSize}
}

// Read implements io.Reader, failing the read once the source has produced
// more bytes than the configured limit allows. A source of exactly maxSize
// bytes still reads to EOF: one byte beyond the limit is what proves the
// source is over it.
func (l *limitedFileReader) Read(p []byte) (int, error) {
	if l.read > l.maxSize {
		return 0, l.tooLarge()
	}
	if allowed := l.maxSize - l.read + 1; int64(len(p)) > allowed {
		p = p[:allowed]
	}
	n, err := l.inner.Read(p)
	l.read += int64(n)
	if l.read > l.maxSize {
		return 0, l.tooLarge()
	}
	return n, err
}

// tooLarge reports the limit refusal in the op-prefixed shape every upload
// input error uses.
func (l *limitedFileReader) tooLarge() error {
	return fmt.Errorf("%s: file exceeds maximum allowed size of %d bytes", l.op, l.maxSize)
}

// decodeBase64Content decodes an inline content_base64 payload and enforces
// the same configured upload size limit that OpenAndValidateFile applies to
// file_path sources, so neither input branch can bypass MaxFileSize. The
// limit is checked against base64.DecodedLen before decoding, so an
// oversized payload is rejected without allocating its decoded form; the
// exact post-decode check covers the padding slack DecodedLen overestimates.
func decodeBase64Content(op, contentBase64 string) ([]byte, error) {
	maxSize := GetUploadConfig().MaxFileSize
	if maxSize > 0 && int64(base64.StdEncoding.DecodedLen(len(contentBase64))) > maxSize+2 {
		return nil, fmt.Errorf("%s: decoded content would exceed maximum allowed size of %d bytes", op, maxSize)
	}
	decoded, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid base64 content: %w", op, err)
	}
	if maxSize > 0 && int64(len(decoded)) > maxSize {
		return nil, fmt.Errorf("%s: decoded content is %d bytes, exceeds maximum allowed size of %d bytes",
			op, len(decoded), maxSize)
	}
	return decoded, nil
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
	decoded, err := decodeBase64Content(op, contentBase64)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(decoded), nil
}
