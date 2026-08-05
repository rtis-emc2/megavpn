package binaryrepo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type ImportRequest struct {
	SourceFile        string
	SourceFilename    string
	Name              string
	Kind              string
	ServiceCode       string
	Version           string
	OSFamily          string
	OSVersion         string
	Architecture      string
	InstallMode       string
	InstallPath       string
	ArchiveBinaryPath string
	Signature         string
	StoragePath       string
	ExpectedSHA256    string
	ReplaceFile       bool
	MaxBytes          int64
}

func ImportFile(root string, req ImportRequest) (domain.BinaryArtifact, error) {
	source := strings.TrimSpace(req.SourceFile)
	if source == "" {
		return domain.BinaryArtifact{}, fmt.Errorf("source file is required")
	}
	info, err := os.Stat(source)
	if err != nil {
		return domain.BinaryArtifact{}, fmt.Errorf("stat source file: %w", err)
	}
	if info.IsDir() {
		return domain.BinaryArtifact{}, fmt.Errorf("source file must not be a directory")
	}
	file, err := os.Open(source)
	if err != nil {
		return domain.BinaryArtifact{}, fmt.Errorf("open source file: %w", err)
	}
	defer file.Close()
	if strings.TrimSpace(req.SourceFilename) == "" {
		req.SourceFilename = filepath.Base(source)
	}
	if strings.TrimSpace(req.Name) == "" {
		req.Name = filepath.Base(source)
	}
	return ImportReader(root, file, req)
}

func ImportReader(root string, reader io.Reader, req ImportRequest) (domain.BinaryArtifact, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return domain.BinaryArtifact{}, fmt.Errorf("artifact root is required")
	}
	if reader == nil {
		return domain.BinaryArtifact{}, fmt.Errorf("artifact reader is required")
	}
	filename := strings.TrimSpace(req.SourceFilename)
	if filename == "" {
		filename = "artifact"
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = filename
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if kind == "" {
		kind = InferKind(filename)
	}
	serviceCode := strings.TrimSpace(req.ServiceCode)
	version := strings.TrimSpace(req.Version)
	if serviceCode == "" {
		return domain.BinaryArtifact{}, fmt.Errorf("service_code is required")
	}
	if version == "" {
		return domain.BinaryArtifact{}, fmt.Errorf("version is required")
	}
	arch := strings.TrimSpace(req.Architecture)
	if arch == "" {
		arch = "amd64"
	}
	storagePath := strings.TrimSpace(req.StoragePath)
	if storagePath == "" {
		storagePath = GeneratedStoragePath(serviceCode, arch, version, kind, filename)
	}
	cleanedStoragePath, err := CleanRelativePath(storagePath)
	if err != nil {
		return domain.BinaryArtifact{}, err
	}
	sha, size, err := CopyArtifact(root, reader, cleanedStoragePath, CopyOptions{
		ReplaceFile:    req.ReplaceFile,
		ExpectedSHA256: req.ExpectedSHA256,
		MaxBytes:       req.MaxBytes,
	})
	if err != nil {
		return domain.BinaryArtifact{}, err
	}
	metadata := map[string]any{}
	if installMode := strings.TrimSpace(req.InstallMode); installMode != "" {
		metadata["install_mode"] = installMode
	}
	if installPath := strings.TrimSpace(req.InstallPath); installPath != "" {
		metadata["install_path"] = installPath
	}
	if archiveBinaryPath := strings.TrimSpace(req.ArchiveBinaryPath); archiveBinaryPath != "" {
		metadata["archive_binary_path"] = archiveBinaryPath
	}
	return domain.BinaryArtifact{
		Name:         name,
		Kind:         kind,
		ServiceCode:  serviceCode,
		Version:      version,
		OSFamily:     strings.TrimSpace(req.OSFamily),
		OSVersion:    strings.TrimSpace(req.OSVersion),
		Architecture: arch,
		StoragePath:  cleanedStoragePath,
		SizeBytes:    size,
		SHA256:       sha,
		Signature:    strings.TrimSpace(req.Signature),
		Status:       "active",
		Metadata:     metadata,
	}, nil
}

func InferKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".sh":
		return "script"
	case ".deb":
		return "package"
	case ".zip":
		return "bundle"
	default:
		return "runtime"
	}
}

func GeneratedStoragePath(serviceCode, arch, version, kind, filename string) string {
	return filepath.ToSlash(filepath.Join(
		"runtime-repository",
		SafePathSegment(serviceCode),
		SafePathSegment(arch),
		SafePathSegment(version),
		SafePathSegment(kind)+"-"+SafePathSegment(filename),
	))
}

func SafePathSegment(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		allowed := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if allowed {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), ".-_")
	if out == "" {
		return "artifact"
	}
	return out
}

func CleanRelativePath(path string) (string, error) {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return "", fmt.Errorf("storage path is required")
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("storage path must be relative to artifact root")
	}
	if strings.Contains(path, "\x00") {
		return "", fmt.Errorf("storage path contains NUL")
	}
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if cleaned == "." || cleaned == "" || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("storage path must not escape artifact root")
	}
	return cleaned, nil
}

type CopyOptions struct {
	ReplaceFile    bool
	ExpectedSHA256 string
	MaxBytes       int64
}

func CopyArtifact(root string, source io.Reader, storagePath string, opts CopyOptions) (string, int64, error) {
	storagePath, err := CleanRelativePath(storagePath)
	if err != nil {
		return "", 0, err
	}
	rootAbs, destination, err := destinationPath(root, storagePath)
	if err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(rootAbs, 0o750); err != nil {
		return "", 0, fmt.Errorf("create artifact root: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return "", 0, fmt.Errorf("create artifact directory: %w", err)
	}
	if err := validateParentWithinRoot(rootAbs, filepath.Dir(destination)); err != nil {
		return "", 0, err
	}
	if !opts.ReplaceFile {
		if _, err := os.Lstat(destination); err == nil {
			return "", 0, fmt.Errorf("artifact file already exists: %s", destination)
		} else if !os.IsNotExist(err) {
			return "", 0, fmt.Errorf("stat artifact file: %w", err)
		}
	}

	tmp, err := os.CreateTemp(filepath.Dir(destination), ".artifact-*")
	if err != nil {
		return "", 0, fmt.Errorf("create artifact temp file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	hash := sha256.New()
	writer := io.MultiWriter(tmp, hash)
	var reader io.Reader = source
	if opts.MaxBytes > 0 {
		reader = &io.LimitedReader{R: source, N: opts.MaxBytes + 1}
	}
	written, copyErr := io.Copy(writer, reader)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		if copyErr != nil {
			return "", 0, fmt.Errorf("copy artifact file: %w", copyErr)
		}
		return "", 0, fmt.Errorf("close artifact file: %w", closeErr)
	}
	if opts.MaxBytes > 0 && written > opts.MaxBytes {
		return "", 0, fmt.Errorf("artifact exceeds maximum upload size")
	}
	if err := os.Chmod(tmpPath, 0o640); err != nil {
		return "", 0, fmt.Errorf("set artifact file mode: %w", err)
	}
	sha := hex.EncodeToString(hash.Sum(nil))
	if expected := strings.ToLower(strings.TrimSpace(opts.ExpectedSHA256)); expected != "" {
		if len(expected) != 64 || strings.Trim(expected, "0123456789abcdef") != "" {
			return "", 0, fmt.Errorf("expected_sha256 must be 64 lowercase hex characters")
		}
		if sha != expected {
			return "", 0, fmt.Errorf("sha256 mismatch: got %s", sha)
		}
	}
	if opts.ReplaceFile {
		if err := os.Rename(tmpPath, destination); err != nil {
			return "", 0, fmt.Errorf("store artifact file: %w", err)
		}
	} else {
		if err := os.Link(tmpPath, destination); err != nil {
			return "", 0, fmt.Errorf("store artifact file: %w", err)
		}
		if err := os.Remove(tmpPath); err != nil {
			return "", 0, fmt.Errorf("remove artifact temp file: %w", err)
		}
	}
	removeTmp = false
	return sha, written, nil
}

func destinationPath(root, storagePath string) (string, string, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return "", "", fmt.Errorf("artifact root is required")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("artifact root is invalid")
	}
	destination := filepath.Join(rootAbs, filepath.FromSlash(storagePath))
	rel, err := filepath.Rel(rootAbs, destination)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("destination escapes artifact root")
	}
	return rootAbs, destination, nil
}

func validateParentWithinRoot(root, parent string) error {
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("artifact root is invalid: %w", err)
	}
	parentReal, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return fmt.Errorf("artifact directory is invalid: %w", err)
	}
	rel, err := filepath.Rel(rootReal, parentReal)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("artifact path is outside artifact root")
	}
	return nil
}

func RemoveStoredArtifact(root string, artifact domain.BinaryArtifact) {
	path := strings.TrimSpace(artifact.StoragePath)
	if path == "" || filepath.IsAbs(path) {
		return
	}
	cleaned, err := CleanRelativePath(path)
	if err != nil {
		return
	}
	_, destination, err := destinationPath(root, cleaned)
	if err != nil {
		return
	}
	_ = os.Remove(destination)
}
