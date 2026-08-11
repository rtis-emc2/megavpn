package backup

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var backupIDPattern = regexp.MustCompile(`^backup-[0-9]{8}T[0-9]{6}Z-[a-f0-9]{8}$`)

var (
	ErrInvalidID = errors.New("invalid backup id")
	ErrIntegrity = errors.New("backup integrity verification failed")
)

type Record struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
	Verified  bool      `json:"verified"`
}

type Manager struct {
	root        string
	databaseDSN string
	run         func(context.Context, *exec.Cmd) error
}

func New(root, databaseDSN string) (*Manager, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("backup root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve backup root: %w", err)
	}
	m := &Manager{root: filepath.Clean(absRoot), databaseDSN: strings.TrimSpace(databaseDSN)}
	m.run = func(ctx context.Context, cmd *exec.Cmd) error {
		return cmd.Run()
	}
	if err := m.ensureRoot(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) Root() string { return m.root }

func (m *Manager) List() ([]Record, error) {
	if err := m.ensureRoot(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || filepath.Ext(entry.Name()) != ".dump" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".dump")
		if !backupIDPattern.MatchString(id) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		checksum, checksumErr := m.readChecksum(id)
		verified := false
		if checksumErr == nil {
			actual, _, actualErr := fileChecksum(filepath.Join(m.root, entry.Name()))
			verified = actualErr == nil && actual == checksum
		}
		out = append(out, Record{ID: id, Filename: entry.Name(), SizeBytes: info.Size(), SHA256: checksum, CreatedAt: info.ModTime().UTC(), Verified: verified})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (m *Manager) Create(ctx context.Context) (Record, error) {
	if strings.TrimSpace(m.databaseDSN) == "" {
		return Record{}, errors.New("database DSN is not configured")
	}
	if err := m.ensureRoot(); err != nil {
		return Record{}, err
	}
	id, err := newBackupID()
	if err != nil {
		return Record{}, err
	}
	dumpPath, err := m.path(id, ".dump")
	if err != nil {
		return Record{}, err
	}
	file, err := os.OpenFile(dumpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Record{}, fmt.Errorf("create backup archive: %w", err)
	}
	cleanup := true
	defer func() {
		_ = file.Close()
		if cleanup {
			_ = os.Remove(dumpPath)
			_ = os.Remove(strings.TrimSuffix(dumpPath, ".dump") + ".sha256")
		}
	}()

	cmd := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--no-owner", "--no-privileges")
	cmd.Env, err = postgresEnvironment(m.databaseDSN)
	if err != nil {
		return Record{}, err
	}
	cmd.Stdout = file
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := m.run(ctx, cmd); err != nil {
		return Record{}, fmt.Errorf("pg_dump failed: %w: %s", err, truncate(stderr.String(), 800))
	}
	if err := file.Sync(); err != nil {
		return Record{}, fmt.Errorf("sync backup archive: %w", err)
	}
	if err := file.Close(); err != nil {
		return Record{}, fmt.Errorf("close backup archive: %w", err)
	}
	checksum, size, err := fileChecksum(dumpPath)
	if err != nil {
		return Record{}, err
	}
	if err := m.writeChecksum(id, checksum); err != nil {
		return Record{}, err
	}
	if _, err := m.Verify(ctx, id); err != nil {
		return Record{}, err
	}
	cleanup = false
	info, err := os.Stat(dumpPath)
	if err != nil {
		return Record{}, err
	}
	return Record{ID: id, Filename: filepath.Base(dumpPath), SizeBytes: size, SHA256: checksum, CreatedAt: info.ModTime().UTC(), Verified: true}, nil
}

func (m *Manager) Verify(ctx context.Context, id string) (Record, error) {
	dumpPath, err := m.path(id, ".dump")
	if err != nil {
		return Record{}, err
	}
	expected, err := m.readChecksum(id)
	if err != nil {
		return Record{}, err
	}
	actual, size, err := fileChecksum(dumpPath)
	if err != nil {
		return Record{}, err
	}
	if actual != expected {
		return Record{}, fmt.Errorf("%w: checksum mismatch", ErrIntegrity)
	}
	cmd := exec.CommandContext(ctx, "pg_restore", "--list", dumpPath)
	cmd.Env = append([]string{}, os.Environ()...)
	var stderr strings.Builder
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := m.run(ctx, cmd); err != nil {
		return Record{}, fmt.Errorf("pg_restore archive validation failed: %w: %s", err, truncate(stderr.String(), 800))
	}
	info, err := os.Stat(dumpPath)
	if err != nil {
		return Record{}, err
	}
	return Record{ID: id, Filename: filepath.Base(dumpPath), SizeBytes: size, SHA256: actual, CreatedAt: info.ModTime().UTC(), Verified: true}, nil

}

func (m *Manager) Open(id string) (*os.File, Record, error) {
	dumpPath, err := m.path(id, ".dump")
	if err != nil {
		return nil, Record{}, err
	}
	expected, err := m.readChecksum(id)
	if err != nil {
		return nil, Record{}, err
	}
	actual, size, err := fileChecksum(dumpPath)
	if err != nil {
		return nil, Record{}, err
	}
	if actual != expected {
		return nil, Record{}, fmt.Errorf("%w: checksum mismatch", ErrIntegrity)
	}
	file, err := os.Open(dumpPath)
	if err != nil {
		return nil, Record{}, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, Record{}, err
	}
	return file, Record{ID: id, Filename: filepath.Base(dumpPath), SizeBytes: size, SHA256: actual, CreatedAt: info.ModTime().UTC(), Verified: true}, nil
}

func (m *Manager) Delete(id string) error {
	dumpPath, err := m.path(id, ".dump")
	if err != nil {
		return err
	}
	if _, err := requireRegularFile(dumpPath); err != nil {
		return err
	}
	if err := os.Remove(dumpPath); err != nil {
		return err
	}
	checksumPath, _ := m.path(id, ".sha256")
	if err := os.Remove(checksumPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (m *Manager) ensureRoot() error {
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return fmt.Errorf("create backup root: %w", err)
	}
	info, err := os.Lstat(m.root)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("backup root must be a real directory")
	}
	if err := os.Chmod(m.root, 0o700); err != nil {
		return fmt.Errorf("secure backup root: %w", err)
	}
	return nil
}

func (m *Manager) path(id, suffix string) (string, error) {
	if !backupIDPattern.MatchString(strings.TrimSpace(id)) {
		return "", ErrInvalidID
	}
	path := filepath.Join(m.root, id+suffix)
	if filepath.Dir(path) != m.root {
		return "", errors.New("backup path escaped configured root")
	}
	return path, nil
}

func (m *Manager) writeChecksum(id, checksum string) error {
	path, err := m.path(id, ".sha256")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.WriteString(file, checksum+"\n"); err != nil {
		return err
	}
	return file.Sync()
}

func (m *Manager) readChecksum(id string) (string, error) {
	path, err := m.path(id, ".sha256")
	if err != nil {
		return "", err
	}
	if _, err := requireRegularFile(path); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, 256))
	if !scanner.Scan() {
		return "", fmt.Errorf("%w: checksum is empty", ErrIntegrity)
	}
	value := strings.TrimSpace(scanner.Text())
	if len(value) != sha256.Size*2 {
		return "", fmt.Errorf("%w: checksum has invalid length", ErrIntegrity)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", fmt.Errorf("%w: checksum is not hexadecimal", ErrIntegrity)
	}
	return value, scanner.Err()
}

func newBackupID() (string, error) {
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate backup id: %w", err)
	}
	return "backup-" + time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(random), nil
}

func fileChecksum(path string) (string, int64, error) {
	if _, err := requireRegularFile(path); err != nil {
		return "", 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func requireRegularFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("backup artifact must be a regular file")
	}
	return info, nil
}

func postgresEnvironment(dsn string) ([]string, error) {
	config, err := pgx.ParseConfig(strings.TrimSpace(dsn))
	if err != nil {
		return nil, fmt.Errorf("parse database DSN: %w", err)
	}
	blocked := map[string]struct{}{
		"PGHOST": {}, "PGPORT": {}, "PGUSER": {}, "PGPASSWORD": {},
		"PGDATABASE": {}, "PGSSLMODE": {}, "PGSERVICE": {}, "PGSERVICEFILE": {},
	}
	env := make([]string, 0, len(os.Environ())+6)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, skip := blocked[key]; !skip {
			env = append(env, entry)
		}
	}
	env = append(env,
		"PGHOST="+config.Host,
		fmt.Sprintf("PGPORT=%d", config.Port),
		"PGUSER="+config.User,
		"PGPASSWORD="+config.Password,
		"PGDATABASE="+config.Database,
	)
	if sslMode := strings.TrimSpace(config.RuntimeParams["sslmode"]); sslMode != "" {
		env = append(env, "PGSSLMODE="+sslMode)
	}
	return env, nil
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}
