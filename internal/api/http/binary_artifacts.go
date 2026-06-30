package http

import (
	"context"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/binaryrepo"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

const maxBinaryArtifactUploadBytes int64 = 512 * 1024 * 1024

func (s *Server) listBinaryArtifacts(w http.ResponseWriter, r *http.Request) {
	includeInactive := r.URL.Query().Get("include_inactive") == "1"
	items, err := s.store.ListBinaryArtifacts(r.Context(), includeInactive)
	if err != nil {
		writeErr(w, 500, "list binary artifacts failed")
		return
	}
	if items == nil {
		items = []domain.BinaryArtifact{}
	}
	writeJSON(w, 200, items)
}

func (s *Server) createBinaryArtifact(w http.ResponseWriter, r *http.Request) {
	var req domain.BinaryArtifact
	if !decode(r, &req) {
		writeErr(w, 400, "invalid binary artifact payload")
		return
	}
	item, err := s.store.CreateBinaryArtifact(r.Context(), req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, item)
}

func (s *Server) importBinaryArtifact(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(s.artifactRoot)
	if root == "" {
		writeErr(w, 500, "artifact root is not configured")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBinaryArtifactUploadBytes+1024*1024)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, 400, "invalid artifact upload form: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "artifact file is required")
		return
	}
	defer file.Close()
	artifact, err := binaryrepo.ImportReader(root, file, binaryrepo.ImportRequest{
		SourceFilename: strings.TrimSpace(header.Filename),
		Name:           formValue(r, "name"),
		Kind:           formValue(r, "kind"),
		ServiceCode:    formValue(r, "service_code"),
		Version:        formValue(r, "version"),
		OSFamily:       formValue(r, "os_family"),
		OSVersion:      formValue(r, "os_version"),
		Architecture:   formValue(r, "architecture"),
		InstallMode:    formValue(r, "install_mode"),
		InstallPath:    formValue(r, "install_path"),
		Signature:      formValue(r, "signature"),
		StoragePath:    formValue(r, "storage_path"),
		ExpectedSHA256: formValue(r, "expected_sha256"),
		ReplaceFile:    formBool(r, "replace_file"),
		MaxBytes:       maxBinaryArtifactUploadBytes,
	})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	item, err := s.store.CreateBinaryArtifact(r.Context(), artifact)
	if err != nil {
		binaryrepo.RemoveStoredArtifact(root, artifact)
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, item)
}

type binaryArtifactURLImportRequest struct {
	SourceURL      string `json:"source_url"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	ServiceCode    string `json:"service_code"`
	Version        string `json:"version"`
	OSFamily       string `json:"os_family"`
	OSVersion      string `json:"os_version"`
	Architecture   string `json:"architecture"`
	InstallMode    string `json:"install_mode"`
	InstallPath    string `json:"install_path"`
	Signature      string `json:"signature"`
	StoragePath    string `json:"storage_path"`
	ExpectedSHA256 string `json:"expected_sha256"`
	ReplaceFile    bool   `json:"replace_file"`
}

func (s *Server) importBinaryArtifactFromURL(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(s.artifactRoot)
	if root == "" {
		writeErr(w, 500, "artifact root is not configured")
		return
	}
	var req binaryArtifactURLImportRequest
	if !decode(r, &req) {
		writeErr(w, 400, "invalid binary artifact URL import payload")
		return
	}
	if strings.TrimSpace(req.ExpectedSHA256) == "" {
		writeErr(w, 400, "expected_sha256 is required for URL imports")
		return
	}
	sourceURL, err := url.Parse(strings.TrimSpace(req.SourceURL))
	if err != nil {
		writeErr(w, 400, "source_url is invalid")
		return
	}
	if err := validateRemoteArtifactURL(sourceURL); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	client := remoteArtifactHTTPClient()
	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, sourceURL.String(), nil)
	if err != nil {
		writeErr(w, 400, "source_url is invalid")
		return
	}
	httpReq.Header.Set("User-Agent", "MegaVPN-runtime-repository/1")
	resp, err := client.Do(httpReq)
	if err != nil {
		writeErr(w, 502, "download source artifact failed: "+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeErr(w, 502, fmt.Sprintf("download source artifact failed: HTTP %d", resp.StatusCode))
		return
	}
	if resp.ContentLength > maxBinaryArtifactUploadBytes {
		writeErr(w, 413, "artifact exceeds maximum upload size")
		return
	}
	artifact, err := binaryrepo.ImportReader(root, resp.Body, binaryrepo.ImportRequest{
		SourceFilename: remoteArtifactFilename(sourceURL, resp.Header.Get("Content-Disposition")),
		Name:           strings.TrimSpace(req.Name),
		Kind:           strings.TrimSpace(req.Kind),
		ServiceCode:    strings.TrimSpace(req.ServiceCode),
		Version:        strings.TrimSpace(req.Version),
		OSFamily:       strings.TrimSpace(req.OSFamily),
		OSVersion:      strings.TrimSpace(req.OSVersion),
		Architecture:   strings.TrimSpace(req.Architecture),
		InstallMode:    strings.TrimSpace(req.InstallMode),
		InstallPath:    strings.TrimSpace(req.InstallPath),
		Signature:      strings.TrimSpace(req.Signature),
		StoragePath:    strings.TrimSpace(req.StoragePath),
		ExpectedSHA256: strings.TrimSpace(req.ExpectedSHA256),
		ReplaceFile:    req.ReplaceFile,
		MaxBytes:       maxBinaryArtifactUploadBytes,
	})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	item, err := s.store.CreateBinaryArtifact(r.Context(), artifact)
	if err != nil {
		binaryrepo.RemoveStoredArtifact(root, artifact)
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, item)
}

func remoteArtifactFilename(sourceURL *url.URL, contentDisposition string) string {
	if _, params, err := mime.ParseMediaType(contentDisposition); err == nil {
		if filename := strings.TrimSpace(params["filename"]); filename != "" {
			return path.Base(filename)
		}
	}
	if sourceURL != nil {
		if filename := path.Base(sourceURL.EscapedPath()); filename != "." && filename != "/" && filename != "" {
			if decoded, err := url.PathUnescape(filename); err == nil && strings.TrimSpace(decoded) != "" {
				return decoded
			}
			return filename
		}
	}
	return "artifact"
}

func remoteArtifactHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	resolver := net.DefaultResolver
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("artifact URL address is invalid")
		}
		ips, err := resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve artifact URL host: %w", err)
		}
		for _, ip := range ips {
			if !isSafeRemoteArtifactIP(ip.IP) {
				continue
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		}
		return nil, fmt.Errorf("artifact URL host resolves only to private or unsafe addresses")
	}
	return &http.Client{
		Timeout:   5 * time.Minute,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many artifact URL redirects")
			}
			return validateRemoteArtifactURL(req.URL)
		},
	}
}

func validateRemoteArtifactURL(value *url.URL) error {
	if value == nil {
		return fmt.Errorf("source_url is required")
	}
	if strings.ToLower(strings.TrimSpace(value.Scheme)) != "https" {
		return fmt.Errorf("source_url must use https")
	}
	if value.User != nil {
		return fmt.Errorf("source_url must not contain userinfo")
	}
	host := strings.TrimSpace(value.Hostname())
	if host == "" {
		return fmt.Errorf("source_url host is required")
	}
	hostLower := strings.ToLower(host)
	if hostLower == "localhost" || strings.HasSuffix(hostLower, ".localhost") {
		return fmt.Errorf("source_url host is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil && !isSafeRemoteArtifactIP(ip) {
		return fmt.Errorf("source_url IP address is not allowed")
	}
	return nil
}

func isSafeRemoteArtifactIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	return addr.IsValid() &&
		!addr.IsUnspecified() &&
		!addr.IsLoopback() &&
		!addr.IsPrivate() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() &&
		!addr.IsMulticast()
}

func formValue(r *http.Request, key string) string {
	return strings.TrimSpace(r.FormValue(key))
}

func formBool(r *http.Request, key string) bool {
	switch strings.ToLower(strings.TrimSpace(r.FormValue(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
