package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	apihttp "github.com/rtis-emc2/megavpn/internal/api/http"
	authn "github.com/rtis-emc2/megavpn/internal/auth"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
	"github.com/rtis-emc2/megavpn/internal/platform/config"
	"github.com/rtis-emc2/megavpn/internal/platform/database"
	platformversion "github.com/rtis-emc2/megavpn/internal/platform/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "reset-password":
		if err := resetPassword(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "megavpn-admin: %v\n", err)
			os.Exit(1)
		}
	case "seed-service-packs":
		if err := seedServicePacks(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "megavpn-admin: %v\n", err)
			os.Exit(1)
		}
	case "import-binary-artifact":
		if err := importBinaryArtifact(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "megavpn-admin: %v\n", err)
			os.Exit(1)
		}
	case "version", "--version", "-version":
		fmt.Println(platformversion.Version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "megavpn-admin: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `MegaVPN administrative maintenance utility

Usage:
  megavpn-admin reset-password [flags]
  megavpn-admin seed-service-packs [flags]
  megavpn-admin import-binary-artifact [flags]
  megavpn-admin version

Commands:
  reset-password  Reset an existing local platform user's password.
  seed-service-packs
                  Insert built-in service pack templates when the catalog table exists.
  import-binary-artifact
                  Copy a runtime artifact into the control-plane artifact root and register it.

`)
}

func resetPassword(args []string) error {
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		databaseDSN = fs.String("database-dsn", "", "PostgreSQL DSN; defaults to MEGAVPN_DATABASE_DSN")
		envFile     = fs.String("env-file", "/etc/megavpn/megavpn.env", "runtime environment file loaded before reading env vars; empty disables file loading")
		login       = fs.String("login", "superadmin", "platform username or email")
		passwordEnv = fs.String("password-env", "MEGAVPN_ADMIN_PASSWORD", "environment variable containing the new password")
		activate    = fs.Bool("activate", false, "set the user status to active after reset")
		timeout     = fs.Duration("timeout", 10*time.Second, "database operation timeout")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := loadAdminEnvFile(*envFile); err != nil {
		return err
	}

	loginValue := strings.TrimSpace(*login)
	if loginValue == "" {
		return fmt.Errorf("login is required")
	}

	password := os.Getenv(strings.TrimSpace(*passwordEnv))
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("new password is required in %s", strings.TrimSpace(*passwordEnv))
	}
	passwordHash, err := authn.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	dsn, err := resolveAdminDatabaseDSN(*databaseDSN)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	store := postgres.New(db.Pool)
	record, err := store.GetPlatformUserForAuth(ctx, loginValue)
	if err != nil {
		return fmt.Errorf("find platform user %q: %w", loginValue, err)
	}
	if err := store.UpdatePlatformUserPassword(ctx, record.User.ID, passwordHash, nil); err != nil {
		return fmt.Errorf("update platform user password: %w", err)
	}
	if *activate && record.User.Status != "active" {
		if _, err := store.UpdatePlatformUserStatus(ctx, record.User.ID, "active", nil); err != nil {
			return fmt.Errorf("activate platform user: %w", err)
		}
		record.User.Status = "active"
	}

	fmt.Printf("password reset completed: username=%s user_id=%s status=%s activate=%t\n", record.User.Username, record.User.ID, record.User.Status, *activate)
	return nil
}

func seedServicePacks(args []string) error {
	fs := flag.NewFlagSet("seed-service-packs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		databaseDSN = fs.String("database-dsn", "", "PostgreSQL DSN; defaults to MEGAVPN_DATABASE_DSN")
		envFile     = fs.String("env-file", "/etc/megavpn/megavpn.env", "runtime environment file loaded before reading env vars; empty disables file loading")
		timeout     = fs.Duration("timeout", 10*time.Second, "database operation timeout")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := loadAdminEnvFile(*envFile); err != nil {
		return err
	}

	dsn, err := resolveAdminDatabaseDSN(*databaseDSN)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	store := postgres.New(db.Pool)
	defaults := apihttp.DefaultServicePackDefinitions()
	if err := store.EnsureDefaultServicePacks(ctx, defaults); err != nil {
		return fmt.Errorf("seed service pack templates: %w", err)
	}
	active, err := store.ListServicePacks(ctx)
	if err != nil {
		return fmt.Errorf("verify service pack templates: %w", err)
	}

	fmt.Printf("service pack defaults seeded: defaults=%d active=%d\n", len(defaults), len(active))
	return nil
}

func importBinaryArtifact(args []string) error {
	fs := flag.NewFlagSet("import-binary-artifact", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		databaseDSN  = fs.String("database-dsn", "", "PostgreSQL DSN; defaults to MEGAVPN_DATABASE_DSN")
		envFile      = fs.String("env-file", "/etc/megavpn/megavpn.env", "runtime environment file loaded before reading env vars; empty disables file loading")
		artifactRoot = fs.String("artifact-root", "", "artifact storage root; defaults to MEGAVPN_ARTIFACT_ROOT")
		sourceFile   = fs.String("file", "", "local artifact file to import")
		name         = fs.String("name", "", "artifact name; defaults to source filename")
		kind         = fs.String("kind", "", "artifact kind: script, package, runtime or bundle; defaults from file extension")
		serviceCode  = fs.String("service-code", "", "runtime service code, for example xray-core or shadowsocks")
		version      = fs.String("version", "", "runtime artifact version")
		osFamily     = fs.String("os-family", "linux", "target OS family")
		osVersion    = fs.String("os-version", "", "target OS version, empty means any")
		architecture = fs.String("architecture", "amd64", "target architecture: amd64 or arm64")
		installMode  = fs.String("install-mode", "", "optional installer mode, for example xray_install_script or deb_package")
		signature    = fs.String("signature", "", "optional detached signature or signature reference")
		storagePath  = fs.String("storage-path", "", "optional relative repository path under artifact root")
		replace      = fs.Bool("replace-file", false, "replace an existing file at the repository storage path")
		timeout      = fs.Duration("timeout", 10*time.Second, "database operation timeout")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := loadAdminEnvFile(*envFile); err != nil {
		return err
	}

	cfg := config.Load()
	root := strings.TrimSpace(*artifactRoot)
	if root == "" {
		root = strings.TrimSpace(cfg.Artifacts.Root)
	}
	if root == "" {
		return fmt.Errorf("artifact root is empty: set MEGAVPN_ARTIFACT_ROOT or pass --artifact-root")
	}
	dsn, err := resolveAdminDatabaseDSN(*databaseDSN)
	if err != nil {
		return err
	}

	artifact, err := prepareBinaryArtifactImport(root, binaryArtifactImportRequest{
		SourceFile:   *sourceFile,
		Name:         *name,
		Kind:         *kind,
		ServiceCode:  *serviceCode,
		Version:      *version,
		OSFamily:     *osFamily,
		OSVersion:    *osVersion,
		Architecture: *architecture,
		InstallMode:  *installMode,
		Signature:    *signature,
		StoragePath:  *storagePath,
		ReplaceFile:  *replace,
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	store := postgres.New(db.Pool)
	store.SetArtifactRoot(root)
	item, err := store.CreateBinaryArtifact(ctx, artifact)
	if err != nil {
		if !*replace {
			_ = os.Remove(filepath.Join(filepath.Clean(root), filepath.FromSlash(artifact.StoragePath)))
		}
		return fmt.Errorf("register binary artifact: %w", err)
	}
	fmt.Printf("binary artifact imported: id=%s service=%s kind=%s version=%s arch=%s storage_path=%s sha256=%s\n",
		item.ID, item.ServiceCode, item.Kind, item.Version, item.Architecture, item.StoragePath, item.SHA256)
	return nil
}

func resolveAdminDatabaseDSN(flagValue string) (string, error) {
	dsn := strings.TrimSpace(flagValue)
	if dsn == "" {
		dsn = strings.TrimSpace(config.Load().Database.DSN)
	}
	if dsn == "" {
		return "", fmt.Errorf("database DSN is empty: set MEGAVPN_DATABASE_DSN, pass --database-dsn, or provide --env-file")
	}
	return dsn, nil
}

func loadAdminEnvFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open env file %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, raw, found := strings.Cut(line, "=")
		if !found {
			return fmt.Errorf("parse env file %s:%d: expected KEY=VALUE", path, lineNo)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("parse env file %s:%d: empty key", path, lineNo)
		}
		value, err := parseAdminEnvValue(raw)
		if err != nil {
			return fmt.Errorf("parse env file %s:%d: %w", path, lineNo, err)
		}
		if existing, exists := os.LookupEnv(key); !exists || strings.TrimSpace(existing) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set env %s from %s:%d: %w", key, path, lineNo, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read env file %s: %w", path, err)
	}
	return nil
}

func parseAdminEnvValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	var out strings.Builder
	for i := 0; i < len(value); {
		switch value[i] {
		case '\'':
			i++
			start := i
			for i < len(value) && value[i] != '\'' {
				i++
			}
			if i >= len(value) {
				return "", fmt.Errorf("unterminated single-quoted value")
			}
			out.WriteString(value[start:i])
			i++
		case '"':
			i++
			for i < len(value) && value[i] != '"' {
				if value[i] == '\\' && i+1 < len(value) {
					i++
				}
				out.WriteByte(value[i])
				i++
			}
			if i >= len(value) {
				return "", fmt.Errorf("unterminated double-quoted value")
			}
			i++
		case '\\':
			if i+1 >= len(value) {
				return "", fmt.Errorf("dangling escape in value")
			}
			i++
			out.WriteByte(value[i])
			i++
		case '#':
			if strings.TrimSpace(value[:i]) == "" || value[i-1] == ' ' || value[i-1] == '\t' {
				return strings.TrimSpace(out.String()), nil
			}
			out.WriteByte(value[i])
			i++
		default:
			out.WriteByte(value[i])
			i++
		}
	}
	return strings.TrimSpace(out.String()), nil
}

type binaryArtifactImportRequest struct {
	SourceFile   string
	Name         string
	Kind         string
	ServiceCode  string
	Version      string
	OSFamily     string
	OSVersion    string
	Architecture string
	InstallMode  string
	Signature    string
	StoragePath  string
	ReplaceFile  bool
}

func prepareBinaryArtifactImport(root string, req binaryArtifactImportRequest) (domain.BinaryArtifact, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return domain.BinaryArtifact{}, fmt.Errorf("artifact root is required")
	}
	source := strings.TrimSpace(req.SourceFile)
	if source == "" {
		return domain.BinaryArtifact{}, fmt.Errorf("--file is required")
	}
	info, err := os.Stat(source)
	if err != nil {
		return domain.BinaryArtifact{}, fmt.Errorf("stat source file: %w", err)
	}
	if info.IsDir() {
		return domain.BinaryArtifact{}, fmt.Errorf("source file must not be a directory")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = filepath.Base(source)
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if kind == "" {
		kind = inferBinaryArtifactKind(source)
	}
	serviceCode := strings.TrimSpace(req.ServiceCode)
	version := strings.TrimSpace(req.Version)
	if serviceCode == "" {
		return domain.BinaryArtifact{}, fmt.Errorf("--service-code is required")
	}
	if version == "" {
		return domain.BinaryArtifact{}, fmt.Errorf("--version is required")
	}
	arch := strings.TrimSpace(req.Architecture)
	if arch == "" {
		arch = "amd64"
	}
	storagePath := strings.TrimSpace(req.StoragePath)
	if storagePath == "" {
		storagePath = generatedBinaryArtifactStoragePath(serviceCode, arch, version, kind, filepath.Base(source))
	}
	storagePath, err = cleanRelativeRepositoryPath(storagePath)
	if err != nil {
		return domain.BinaryArtifact{}, err
	}
	sha, size, err := copyBinaryArtifactFile(root, source, storagePath, req.ReplaceFile)
	if err != nil {
		return domain.BinaryArtifact{}, err
	}
	metadata := map[string]any{}
	if installMode := strings.TrimSpace(req.InstallMode); installMode != "" {
		metadata["install_mode"] = installMode
	}
	return domain.BinaryArtifact{
		Name:         name,
		Kind:         kind,
		ServiceCode:  serviceCode,
		Version:      version,
		OSFamily:     strings.TrimSpace(req.OSFamily),
		OSVersion:    strings.TrimSpace(req.OSVersion),
		Architecture: arch,
		StoragePath:  storagePath,
		SizeBytes:    size,
		SHA256:       sha,
		Signature:    strings.TrimSpace(req.Signature),
		Status:       "active",
		Metadata:     metadata,
	}, nil
}

func inferBinaryArtifactKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".sh":
		return "script"
	case ".deb":
		return "package"
	default:
		return "runtime"
	}
}

func generatedBinaryArtifactStoragePath(serviceCode, arch, version, kind, filename string) string {
	return filepath.ToSlash(filepath.Join(
		"runtime-repository",
		safeArtifactPathSegment(serviceCode),
		safeArtifactPathSegment(arch),
		safeArtifactPathSegment(version),
		safeArtifactPathSegment(kind)+"-"+safeArtifactPathSegment(filename),
	))
}

func safeArtifactPathSegment(value string) string {
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

func cleanRelativeRepositoryPath(path string) (string, error) {
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

func copyBinaryArtifactFile(root, source, storagePath string, replace bool) (string, int64, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	destination := filepath.Join(root, filepath.FromSlash(storagePath))
	rel, err := filepath.Rel(root, destination)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", 0, fmt.Errorf("destination escapes artifact root")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return "", 0, fmt.Errorf("create artifact directory: %w", err)
	}
	if !replace {
		if _, err := os.Stat(destination); err == nil {
			return "", 0, fmt.Errorf("artifact file already exists: %s", destination)
		} else if !os.IsNotExist(err) {
			return "", 0, fmt.Errorf("stat artifact file: %w", err)
		}
	}

	src, err := os.Open(source)
	if err != nil {
		return "", 0, fmt.Errorf("open source file: %w", err)
	}
	defer src.Close()

	tmp := destination + ".tmp-" + fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	dst, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return "", 0, fmt.Errorf("create artifact file: %w", err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(dst, hash), src)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		if copyErr != nil {
			return "", 0, fmt.Errorf("copy artifact file: %w", copyErr)
		}
		return "", 0, fmt.Errorf("close artifact file: %w", closeErr)
	}
	if replace {
		if err := os.Rename(tmp, destination); err != nil {
			_ = os.Remove(tmp)
			return "", 0, fmt.Errorf("store artifact file: %w", err)
		}
	} else {
		if err := os.Link(tmp, destination); err != nil {
			_ = os.Remove(tmp)
			return "", 0, fmt.Errorf("store artifact file: %w", err)
		}
		_ = os.Remove(tmp)
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}
