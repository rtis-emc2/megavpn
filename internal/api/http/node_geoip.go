package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

const (
	nodeGeoIPDefaultTimeout   = 3 * time.Second
	nodeGeoIPRefreshAfter     = 30 * 24 * time.Hour
	nodeGeoIPMaxResponseBytes = 256 * 1024
)

type nodeGeoIPResolver struct {
	urlTemplate string
	provider    string
	timeout     time.Duration
	client      *nethttp.Client
}

func newNodeGeoIPResolver(urlTemplate string, timeout time.Duration) *nodeGeoIPResolver {
	urlTemplate = strings.TrimSpace(urlTemplate)
	if urlTemplate == "" || strings.EqualFold(urlTemplate, "disabled") || strings.EqualFold(urlTemplate, "off") {
		return nil
	}
	if timeout <= 0 {
		timeout = nodeGeoIPDefaultTimeout
	}
	provider := "geoip"
	if parsed, err := url.Parse(urlTemplate); err == nil && parsed.Hostname() != "" {
		provider = parsed.Hostname()
	}
	return &nodeGeoIPResolver{
		urlTemplate: urlTemplate,
		provider:    provider,
		timeout:     timeout,
		client: &nethttp.Client{
			Timeout: timeout,
			Transport: &nethttp.Transport{
				Proxy:                 nethttp.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext,
				TLSHandshakeTimeout:   timeout,
				ResponseHeaderTimeout: timeout,
				IdleConnTimeout:       30 * time.Second,
				MaxIdleConns:          4,
				MaxIdleConnsPerHost:   2,
			},
		},
	}
}

func (r *nodeGeoIPResolver) Lookup(ctx context.Context, address string) domain.NodeGeoIP {
	if r == nil {
		return nodeGeoIPSkipped("geoip resolver is disabled")
	}
	ip, err := publicNodeIP(ctx, address)
	if err != nil {
		out := nodeGeoIPSkipped(err.Error())
		out.Provider = r.provider
		return out
	}
	lookupURL, err := r.lookupURL(ip)
	if err != nil {
		out := nodeGeoIPFailed(r.provider, ip.String(), err)
		return out
	}
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, lookupURL, nil)
	if err != nil {
		return nodeGeoIPFailed(r.provider, ip.String(), err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return nodeGeoIPFailed(r.provider, ip.String(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nodeGeoIPFailed(r.provider, ip.String(), fmt.Errorf("geoip provider returned status %d", resp.StatusCode))
	}
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, nodeGeoIPMaxResponseBytes)).Decode(&payload); err != nil {
		return nodeGeoIPFailed(r.provider, ip.String(), err)
	}
	out, err := parseNodeGeoIPResponse(r.provider, ip.String(), payload)
	if err != nil {
		return nodeGeoIPFailed(r.provider, ip.String(), err)
	}
	return out
}

func (r *nodeGeoIPResolver) lookupURL(ip netip.Addr) (string, error) {
	if !strings.Contains(r.urlTemplate, "{ip}") {
		return "", errors.New("geoip URL template must contain {ip}")
	}
	raw := strings.ReplaceAll(r.urlTemplate, "{ip}", url.PathEscape(ip.String()))
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "https" {
		return "", errors.New("geoip URL template must use https")
	}
	if parsed.User != nil || parsed.Hostname() == "" {
		return "", errors.New("geoip URL template host is invalid")
	}
	return parsed.String(), nil
}

func parseNodeGeoIPResponse(provider, fallbackIP string, payload map[string]any) (domain.NodeGeoIP, error) {
	lat, okLat := jsonNumber(payload, "latitude", "lat")
	lon, okLon := jsonNumber(payload, "longitude", "lon", "lng")
	if !okLat || !okLon {
		if loc := jsonString(payload, "loc"); loc != "" {
			parts := strings.Split(loc, ",")
			if len(parts) == 2 {
				lat, okLat = parseFloat(parts[0])
				lon, okLon = parseFloat(parts[1])
			}
		}
	}
	if !okLat || !okLon {
		return domain.NodeGeoIP{}, errors.New("geoip response did not include coordinates")
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return domain.NodeGeoIP{}, errors.New("geoip response coordinates are out of range")
	}
	now := time.Now().UTC()
	ip := firstNonEmptyGeoIP(jsonString(payload, "ip", "query"), fallbackIP)
	countryCode := strings.ToUpper(jsonString(payload, "country_code", "countryCode", "country"))
	countryName := jsonString(payload, "country_name", "country")
	if countryName == countryCode {
		countryName = ""
	}
	region := jsonString(payload, "region", "regionName")
	city := jsonString(payload, "city")
	org := firstNonEmptyGeoIP(jsonString(payload, "org", "organization", "isp"), jsonString(payload, "as"))
	asn := jsonString(payload, "asn")
	if asn == "" {
		asn = parseASN(jsonString(payload, "as", "org"))
	}
	label := nodeGeoIPLocationLabel(city, region, countryName, countryCode, org)
	return domain.NodeGeoIP{
		Provider:         strings.TrimSpace(provider),
		Status:           "resolved",
		IP:               ip,
		CountryCode:      countryCode,
		CountryName:      countryName,
		Region:           region,
		City:             city,
		Org:              org,
		ASN:              asn,
		LocationLabel:    label,
		Latitude:         &lat,
		Longitude:        &lon,
		AccuracyRadiusKM: nil,
		ResolvedAt:       &now,
		Error:            "",
	}, nil
}

func nodeGeoIPLocationLabel(parts ...string) string {
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := strings.ToLower(part)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, part)
	}
	return strings.Join(out, ", ")
}

func jsonString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := payload[key].(type) {
		case string:
			if v := strings.TrimSpace(value); v != "" {
				return v
			}
		case json.Number:
			return value.String()
		case float64:
			return fmt.Sprintf("%.0f", value)
		}
	}
	return ""
}

func jsonNumber(payload map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		switch value := payload[key].(type) {
		case float64:
			return value, true
		case json.Number:
			if n, ok := parseFloat(value.String()); ok {
				return n, true
			}
		case string:
			if n, ok := parseFloat(value); ok {
				return n, true
			}
		}
	}
	return 0, false
}

func parseFloat(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	var out float64
	if n, err := strconv.ParseFloat(value, 64); err == nil {
		out = n
	} else {
		return 0, false
	}
	return out, true
}

func firstNonEmptyGeoIP(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func parseASN(value string) string {
	for _, field := range strings.Fields(strings.TrimSpace(value)) {
		field = strings.Trim(field, ",;")
		upper := strings.ToUpper(field)
		if strings.HasPrefix(upper, "AS") && len(upper) > 2 {
			return upper
		}
	}
	return ""
}

func nodeGeoIPSkipped(message string) domain.NodeGeoIP {
	now := time.Now().UTC()
	return domain.NodeGeoIP{
		Status:     "skipped",
		ResolvedAt: &now,
		Error:      strings.TrimSpace(message),
	}
}

func nodeGeoIPFailed(provider, ip string, err error) domain.NodeGeoIP {
	now := time.Now().UTC()
	return domain.NodeGeoIP{
		Provider:   strings.TrimSpace(provider),
		Status:     "failed",
		IP:         strings.TrimSpace(ip),
		ResolvedAt: &now,
		Error:      strings.TrimSpace(err.Error()),
	}
}

func publicNodeIP(ctx context.Context, address string) (netip.Addr, error) {
	host := nodeAddressHost(address)
	if host == "" {
		return netip.Addr{}, errors.New("node address host is empty")
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if !isPublicNodeAddr(addr) {
			return netip.Addr{}, fmt.Errorf("node address %s is not a public IP", addr.String())
		}
		return addr, nil
	}
	ctx, cancel := context.WithTimeout(ctx, nodeGeoIPDefaultTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("resolve node address %q: %w", host, err)
	}
	for _, addr := range addrs {
		if isPublicNodeAddr(addr) {
			return addr, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("node address %q does not resolve to a public IP", host)
}

func nodeAddressHost(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if parsed, err := url.Parse(address); err == nil && parsed.Hostname() != "" {
		return strings.TrimSpace(parsed.Hostname())
	}
	if host, _, err := net.SplitHostPort(address); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(address, "[]")
}

func isPublicNodeAddr(addr netip.Addr) bool {
	return addr.IsValid() &&
		!addr.IsUnspecified() &&
		!addr.IsLoopback() &&
		!addr.IsPrivate() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() &&
		!addr.IsMulticast()
}

func (s *Server) enrichNodeGeoIP(ctx context.Context, nodes []domain.Node) []domain.Node {
	if s.geoIPResolver == nil || len(nodes) == 0 {
		return nodes
	}
	limit := s.geoIPAutoEnrichLimit
	if limit <= 0 {
		limit = 5
	}
	updated := 0
	for idx := range nodes {
		if updated >= limit {
			break
		}
		if !s.nodeNeedsGeoIPRefresh(ctx, nodes[idx]) {
			continue
		}
		geo := s.geoIPResolver.Lookup(ctx, nodes[idx].Address)
		next, err := s.store.UpdateNodeGeoIP(ctx, nodes[idx].ID, geo)
		if err != nil {
			if s.log != nil {
				s.log.Warn("node geoip cache update failed", "node_id", nodes[idx].ID, "error", err)
			}
			continue
		}
		nodes[idx] = next
		updated++
	}
	return nodes
}

func (s *Server) nodeNeedsGeoIPRefresh(ctx context.Context, n domain.Node) bool {
	if strings.TrimSpace(n.Address) == "" {
		return false
	}
	ip, err := publicNodeIP(ctx, n.Address)
	if err != nil {
		return strings.TrimSpace(n.GeoIPStatus) != "skipped"
	}
	if n.Latitude == nil || n.Longitude == nil || strings.TrimSpace(n.GeoIPIP) != ip.String() {
		return true
	}
	if strings.TrimSpace(n.GeoIPStatus) != "resolved" {
		return true
	}
	return n.GeoIPResolvedAt == nil || time.Since(*n.GeoIPResolvedAt) > nodeGeoIPRefreshAfter
}

func (s *Server) resolveNodeGeoIPForProfile(ctx context.Context, n *domain.Node) {
	if n == nil {
		return
	}
	if s.geoIPResolver == nil {
		applyNodeGeoIP(n, nodeGeoIPSkipped("geoip resolver is disabled"))
		return
	}
	applyNodeGeoIP(n, s.geoIPResolver.Lookup(ctx, n.Address))
}

func applyNodeGeoIP(n *domain.Node, geo domain.NodeGeoIP) {
	n.LocationLabel = geo.LocationLabel
	n.Latitude = geo.Latitude
	n.Longitude = geo.Longitude
	n.AccuracyRadiusKM = geo.AccuracyRadiusKM
	n.GeoIPProvider = geo.Provider
	n.GeoIPStatus = geo.Status
	n.GeoIPIP = geo.IP
	n.GeoIPCountryCode = geo.CountryCode
	n.GeoIPCountryName = geo.CountryName
	n.GeoIPRegion = geo.Region
	n.GeoIPCity = geo.City
	n.GeoIPOrg = geo.Org
	n.GeoIPASN = geo.ASN
	n.GeoIPResolvedAt = geo.ResolvedAt
	n.GeoIPError = geo.Error
}
