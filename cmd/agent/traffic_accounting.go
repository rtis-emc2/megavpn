package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	xrayStatsCollectorName         = "xray_stats"
	wireGuardTransferCollectorName = "wireguard_transfer"
	openVPNStatusCollectorName     = "openvpn_status"
	xrayStatsQueryPattern          = "user>>>"
)

var (
	xrayStatsTextNamePattern  = regexp.MustCompile(`name:\s*"([^"]+)"`)
	xrayStatsTextValuePattern = regexp.MustCompile(`value:\s*([0-9]+)`)
)

type xrayStatsEndpoint struct {
	Server string
	Tag    string
}

type xrayTrafficReading struct {
	InstanceID   string
	InstanceSlug string
	Endpoint     string
	User         string
	Direction    string
	Value        int64
}

type xrayTrafficAggregate struct {
	InstanceID   string
	InstanceSlug string
	Endpoint     string
	User         string
	RxBytes      int64
	TxBytes      int64
}

type xrayStatsRecord struct {
	Name  string
	Value int64
}

type wireGuardTrafficReading struct {
	InstanceID      string
	InstanceSlug    string
	InterfaceName   string
	PublicKey       string
	ClientAddress   string
	ServiceAccessID string
	User            string
	RxBytes         int64
	TxBytes         int64
}

type wireGuardPeerMetadata struct {
	PublicKey       string
	AllowedIPs      []string
	ServiceAccessID string
	User            string
}

type wireGuardTransferRecord struct {
	PublicKey string
	RxBytes   int64
	TxBytes   int64
}

type openVPNTrafficReading struct {
	InstanceID   string
	InstanceSlug string
	StatusPath   string
	CommonName   string
	RxBytes      int64
	TxBytes      int64
}

type openVPNStatusClient struct {
	CommonName string
	RxBytes    int64
	TxBytes    int64
}

func (c *client) reportTrafficAccounting(ctx context.Context, nodeID string) error {
	if c.trafficReportInterval <= 0 {
		c.trafficReportInterval = time.Minute
	}
	now := time.Now().UTC()
	if !c.lastTrafficReportAt.IsZero() && now.Sub(c.lastTrafficReportAt) < c.trafficReportInterval {
		return nil
	}

	targets, err := c.listRuntimeTargets(ctx, nodeID)
	if err != nil {
		return err
	}
	xrayReadings, xrayErr := collectXrayTrafficReadings(ctx, targets)
	wireGuardReadings, wireGuardErr := collectWireGuardTrafficReadings(ctx, targets)
	openVPNReadings, openVPNErr := collectOpenVPNTrafficReadings(targets)
	collectorErr := joinTrafficAccountingErrors(xrayErr, wireGuardErr, openVPNErr)
	if collectorErr != nil && len(xrayReadings) == 0 && len(wireGuardReadings) == 0 && len(openVPNReadings) == 0 {
		c.lastTrafficReportAt = now
		return collectorErr
	}
	bucketStart := c.lastTrafficReportAt
	if bucketStart.IsZero() {
		bucketStart = now.Add(-c.trafficReportInterval)
	}
	samples := make([]trafficAccountingSample, 0)
	samples = append(samples, c.xrayTrafficAccountingSamples(xrayReadings, bucketStart, now)...)
	samples = append(samples, c.wireGuardTrafficAccountingSamples(wireGuardReadings, bucketStart, now)...)
	samples = append(samples, c.openVPNTrafficAccountingSamples(openVPNReadings, bucketStart, now)...)
	if len(samples) == 0 {
		c.commitXrayTrafficReadings(xrayReadings)
		c.commitWireGuardTrafficReadings(wireGuardReadings)
		c.commitOpenVPNTrafficReadings(openVPNReadings)
		c.lastTrafficReportAt = now
		return collectorErr
	}
	if submitErr := c.submitTrafficAccounting(ctx, nodeID, samples); submitErr != nil {
		return submitErr
	}
	c.commitXrayTrafficReadings(xrayReadings)
	c.commitWireGuardTrafficReadings(wireGuardReadings)
	c.commitOpenVPNTrafficReadings(openVPNReadings)
	c.lastTrafficReportAt = now
	return collectorErr
}

func collectXrayTrafficReadings(ctx context.Context, targets []instanceRuntimeTarget) ([]xrayTrafficReading, error) {
	readings := make([]xrayTrafficReading, 0)
	var errs []string
	for _, target := range targets {
		if normalizeServiceCode(target.ServiceCode) != "xray-core" {
			continue
		}
		if !target.DesiredEnabled || strings.EqualFold(target.DesiredStatus, "deleted") {
			continue
		}
		endpoint, ok, err := loadXrayStatsEndpoint(target.ConfigPath)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", target.Slug, err))
			continue
		}
		if !ok {
			continue
		}
		output, err := queryXrayStats(ctx, endpoint.Server)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", target.Slug, err))
			continue
		}
		records := parseXrayStatsQueryOutput(output)
		for _, record := range records {
			user, direction, ok := parseXrayUserTrafficStatName(record.Name)
			if !ok || record.Value < 0 {
				continue
			}
			readings = append(readings, xrayTrafficReading{
				InstanceID:   strings.TrimSpace(target.InstanceID),
				InstanceSlug: strings.TrimSpace(target.Slug),
				Endpoint:     endpoint.Server,
				User:         user,
				Direction:    direction,
				Value:        record.Value,
			})
		}
	}
	if len(errs) > 0 {
		return readings, errors.New(strings.Join(errs, "; "))
	}
	return readings, nil
}

func loadXrayStatsEndpoint(configPath string) (xrayStatsEndpoint, bool, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return xrayStatsEndpoint{}, false, nil
	}
	body, err := os.ReadFile(configPath)
	if err != nil {
		return xrayStatsEndpoint{}, false, err
	}
	var cfg map[string]any
	if err := json.Unmarshal(body, &cfg); err != nil {
		return xrayStatsEndpoint{}, false, err
	}
	api, _ := cfg["api"].(map[string]any)
	apiTag := strings.TrimSpace(stringify(api["tag"]))
	if apiTag == "" {
		return xrayStatsEndpoint{}, false, nil
	}
	if !xrayAPIHasStatsService(api["services"]) {
		return xrayStatsEndpoint{}, false, nil
	}
	candidateTags := xrayStatsInboundCandidateTags(cfg, apiTag)
	inbounds, _ := cfg["inbounds"].([]any)
	for _, item := range inbounds {
		inbound, _ := item.(map[string]any)
		inboundTag := strings.TrimSpace(stringify(inbound["tag"]))
		if inbound == nil || !candidateTags[inboundTag] {
			continue
		}
		listen := strings.TrimSpace(stringify(inbound["listen"]))
		if listen == "" {
			listen = "127.0.0.1"
		}
		if listen != "127.0.0.1" && listen != "localhost" {
			return xrayStatsEndpoint{}, false, fmt.Errorf("xray stats API inbound %q is not loopback-bound", apiTag)
		}
		port := intFromConfigAny(inbound["port"])
		if port <= 0 || port > 65535 {
			return xrayStatsEndpoint{}, false, fmt.Errorf("xray stats API inbound %q has invalid port", inboundTag)
		}
		return xrayStatsEndpoint{Server: fmt.Sprintf("127.0.0.1:%d", port), Tag: inboundTag}, true, nil
	}
	return xrayStatsEndpoint{}, false, nil
}

func xrayStatsInboundCandidateTags(cfg map[string]any, apiTag string) map[string]bool {
	tags := map[string]bool{apiTag: true}
	routing, _ := cfg["routing"].(map[string]any)
	rules, _ := routing["rules"].([]any)
	for _, item := range rules {
		rule, _ := item.(map[string]any)
		if rule == nil || strings.TrimSpace(stringify(rule["outboundTag"])) != apiTag {
			continue
		}
		switch inboundTag := rule["inboundTag"].(type) {
		case []any:
			for _, tag := range inboundTag {
				if text := strings.TrimSpace(stringify(tag)); text != "" {
					tags[text] = true
				}
			}
		case []string:
			for _, tag := range inboundTag {
				if text := strings.TrimSpace(tag); text != "" {
					tags[text] = true
				}
			}
		case string:
			if text := strings.TrimSpace(inboundTag); text != "" {
				tags[text] = true
			}
		}
	}
	return tags
}

func xrayAPIHasStatsService(raw any) bool {
	switch x := raw.(type) {
	case []any:
		for _, item := range x {
			if strings.EqualFold(strings.TrimSpace(stringify(item)), "StatsService") {
				return true
			}
		}
	case []string:
		for _, item := range x {
			if strings.EqualFold(strings.TrimSpace(item), "StatsService") {
				return true
			}
		}
	case string:
		return strings.EqualFold(strings.TrimSpace(x), "StatsService")
	}
	return false
}

func queryXrayStats(parent context.Context, server string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "xray", "api", "statsquery", "--server="+server, "-pattern", xrayStatsQueryPattern)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("xray statsquery failed: %v: %s", err, truncate(string(output), 1000))
	}
	return string(output), nil
}

func parseXrayStatsQueryOutput(output string) []xrayStatsRecord {
	if records := parseXrayStatsQueryJSON(output); len(records) > 0 {
		return records
	}
	records := make([]xrayStatsRecord, 0)
	currentName := ""
	for _, line := range strings.Split(output, "\n") {
		if match := xrayStatsTextNamePattern.FindStringSubmatch(line); len(match) == 2 {
			currentName = strings.TrimSpace(match[1])
			continue
		}
		if match := xrayStatsTextValuePattern.FindStringSubmatch(line); len(match) == 2 && currentName != "" {
			value, err := strconv.ParseInt(match[1], 10, 64)
			if err == nil {
				records = append(records, xrayStatsRecord{Name: currentName, Value: value})
			}
			currentName = ""
		}
	}
	return records
}

func parseXrayStatsQueryJSON(output string) []xrayStatsRecord {
	output = strings.TrimSpace(output)
	if output == "" || (!strings.HasPrefix(output, "{") && !strings.HasPrefix(output, "[")) {
		return nil
	}
	var doc struct {
		Stat  []map[string]any `json:"stat"`
		Stats []map[string]any `json:"stats"`
	}
	if err := json.Unmarshal([]byte(output), &doc); err != nil {
		return nil
	}
	items := doc.Stat
	if len(items) == 0 {
		items = doc.Stats
	}
	records := make([]xrayStatsRecord, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(stringify(item["name"]))
		value := int64FromAny(item["value"])
		if name == "" || value < 0 {
			continue
		}
		records = append(records, xrayStatsRecord{Name: name, Value: value})
	}
	return records
}

func parseXrayUserTrafficStatName(name string) (string, string, bool) {
	const prefix = "user>>>"
	const separator = ">>>traffic>>>"
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, prefix) {
		return "", "", false
	}
	idx := strings.LastIndex(name, separator)
	if idx < len(prefix) {
		return "", "", false
	}
	user := strings.TrimSpace(name[len(prefix):idx])
	direction := strings.TrimSpace(name[idx+len(separator):])
	if user == "" {
		return "", "", false
	}
	switch direction {
	case "uplink", "downlink":
		return user, direction, true
	default:
		return "", "", false
	}
}

func collectWireGuardTrafficReadings(ctx context.Context, targets []instanceRuntimeTarget) ([]wireGuardTrafficReading, error) {
	readings := make([]wireGuardTrafficReading, 0)
	var errs []string
	for _, target := range targets {
		if normalizeServiceCode(target.ServiceCode) != "wireguard" {
			continue
		}
		if !target.DesiredEnabled || strings.EqualFold(target.DesiredStatus, "deleted") {
			continue
		}
		iface := wireGuardInterfaceNameFromTarget(target)
		if iface == "" {
			errs = append(errs, fmt.Sprintf("%s: wireguard interface name is unknown", target.Slug))
			continue
		}
		peers, err := loadWireGuardPeerMetadata(target.ConfigPath)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", target.Slug, err))
			continue
		}
		output, err := queryWireGuardTransfer(ctx, iface)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", target.Slug, err))
			continue
		}
		for _, record := range parseWireGuardTransferOutput(output) {
			if record.PublicKey == "" || record.RxBytes < 0 || record.TxBytes < 0 {
				continue
			}
			peer := peers[record.PublicKey]
			readings = append(readings, wireGuardTrafficReading{
				InstanceID:      strings.TrimSpace(target.InstanceID),
				InstanceSlug:    strings.TrimSpace(target.Slug),
				InterfaceName:   iface,
				PublicKey:       record.PublicKey,
				ClientAddress:   firstWireGuardAllowedIP(peer.AllowedIPs),
				ServiceAccessID: peer.ServiceAccessID,
				User:            peer.User,
				RxBytes:         record.RxBytes,
				TxBytes:         record.TxBytes,
			})
		}
	}
	if len(errs) > 0 {
		return readings, errors.New(strings.Join(errs, "; "))
	}
	return readings, nil
}

func wireGuardInterfaceNameFromTarget(target instanceRuntimeTarget) string {
	unit := strings.TrimSuffix(strings.TrimSpace(target.SystemdUnit), ".service")
	if strings.HasPrefix(unit, "wg-quick@") {
		iface := strings.TrimSpace(strings.TrimPrefix(unit, "wg-quick@"))
		if iface != "" {
			return iface
		}
	}
	configPath := strings.TrimSpace(target.ConfigPath)
	if configPath == "" {
		return ""
	}
	base := filepath.Base(configPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func loadWireGuardPeerMetadata(configPath string) (map[string]wireGuardPeerMetadata, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return map[string]wireGuardPeerMetadata{}, nil
	}
	body, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]wireGuardPeerMetadata{}, nil
		}
		return nil, err
	}
	return parseWireGuardPeerMetadata(string(body)), nil
}

func parseWireGuardPeerMetadata(config string) map[string]wireGuardPeerMetadata {
	peers := map[string]wireGuardPeerMetadata{}
	var current wireGuardPeerMetadata
	inPeer := false
	pendingComments := map[string]string{}
	flush := func() {
		if inPeer && strings.TrimSpace(current.PublicKey) != "" {
			current.PublicKey = strings.TrimSpace(current.PublicKey)
			current.ClientAddressNormalize()
			peers[current.PublicKey] = current
		}
	}
	for _, rawLine := range strings.Split(config, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			key, value, ok := parseWireGuardManagedComment(line)
			if ok {
				if inPeer {
					assignWireGuardManagedComment(&current, key, value)
				} else {
					pendingComments[key] = value
				}
			}
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flush()
			section := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")))
			inPeer = section == "peer"
			current = wireGuardTrafficPeerFromComments(pendingComments)
			pendingComments = map[string]string{}
			continue
		}
		if !inPeer {
			continue
		}
		key, value, ok := splitConfigAssignment(line)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "publickey":
			current.PublicKey = value
		case "allowedips":
			current.AllowedIPs = splitWireGuardAllowedIPs(value)
		}
	}
	flush()
	return peers
}

func (m *wireGuardPeerMetadata) ClientAddressNormalize() {
	for idx := range m.AllowedIPs {
		m.AllowedIPs[idx] = strings.TrimSpace(m.AllowedIPs[idx])
	}
}

func wireGuardTrafficPeerFromComments(comments map[string]string) wireGuardPeerMetadata {
	var peer wireGuardPeerMetadata
	for key, value := range comments {
		assignWireGuardManagedComment(&peer, key, value)
	}
	return peer
}

func parseWireGuardManagedComment(line string) (string, string, bool) {
	line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#;"))
	if !strings.HasPrefix(line, "megavpn-") {
		return "", "", false
	}
	key, value, ok := splitConfigAssignment(line)
	if !ok {
		return "", "", false
	}
	switch key {
	case "megavpn-service-access-id", "megavpn-client":
		return key, value, true
	default:
		return "", "", false
	}
}

func assignWireGuardManagedComment(peer *wireGuardPeerMetadata, key, value string) {
	switch key {
	case "megavpn-service-access-id":
		peer.ServiceAccessID = strings.TrimSpace(value)
	case "megavpn-client":
		peer.User = strings.TrimSpace(value)
	}
}

func splitConfigAssignment(line string) (string, string, bool) {
	idx := strings.Index(line, "=")
	if idx < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+1:])
	if key == "" {
		return "", "", false
	}
	return key, value, true
}

func splitWireGuardAllowedIPs(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if text := strings.TrimSpace(part); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func firstWireGuardAllowedIP(values []string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func queryWireGuardTransfer(parent context.Context, iface string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wg", "show", iface, "transfer")
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("wg transfer query failed: %v: %s", err, truncate(string(output), 1000))
	}
	return string(output), nil
}

func parseWireGuardTransferOutput(output string) []wireGuardTransferRecord {
	records := make([]wireGuardTransferRecord, 0)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		rx, rxErr := strconv.ParseInt(fields[1], 10, 64)
		tx, txErr := strconv.ParseInt(fields[2], 10, 64)
		if rxErr != nil || txErr != nil {
			continue
		}
		records = append(records, wireGuardTransferRecord{
			PublicKey: strings.TrimSpace(fields[0]),
			RxBytes:   rx,
			TxBytes:   tx,
		})
	}
	return records
}

func collectOpenVPNTrafficReadings(targets []instanceRuntimeTarget) ([]openVPNTrafficReading, error) {
	readings := make([]openVPNTrafficReading, 0)
	var errs []string
	for _, target := range targets {
		if normalizeServiceCode(target.ServiceCode) != "openvpn" {
			continue
		}
		if !target.DesiredEnabled || strings.EqualFold(target.DesiredStatus, "deleted") {
			continue
		}
		statusPath, ok, err := loadOpenVPNStatusPath(target.ConfigPath)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", target.Slug, err))
			continue
		}
		if !ok {
			continue
		}
		body, err := os.ReadFile(statusPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errs = append(errs, fmt.Sprintf("%s: %v", target.Slug, err))
			continue
		}
		for _, client := range parseOpenVPNStatusClients(string(body)) {
			if client.CommonName == "" || client.RxBytes < 0 || client.TxBytes < 0 {
				continue
			}
			readings = append(readings, openVPNTrafficReading{
				InstanceID:   strings.TrimSpace(target.InstanceID),
				InstanceSlug: strings.TrimSpace(target.Slug),
				StatusPath:   statusPath,
				CommonName:   client.CommonName,
				RxBytes:      client.RxBytes,
				TxBytes:      client.TxBytes,
			})
		}
	}
	if len(errs) > 0 {
		return readings, errors.New(strings.Join(errs, "; "))
	}
	return readings, nil
}

func loadOpenVPNStatusPath(configPath string) (string, bool, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return "", false, nil
	}
	body, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	for _, rawLine := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "status" {
			statusPath := strings.Trim(fields[1], `"'`)
			if statusPath == "" {
				continue
			}
			if !filepath.IsAbs(statusPath) {
				statusPath = filepath.Join(filepath.Dir(configPath), statusPath)
			}
			return statusPath, true, nil
		}
	}
	return "", false, nil
}

func parseOpenVPNStatusClients(status string) []openVPNStatusClient {
	reader := csv.NewReader(strings.NewReader(status))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil
	}
	aggregates := map[string]openVPNStatusClient{}
	clientHeader := map[string]int{}
	v1ClientHeader := map[string]int{}
	inV1ClientList := false
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		for idx := range row {
			row[idx] = strings.TrimSpace(row[idx])
		}
		switch row[0] {
		case "HEADER":
			if len(row) >= 3 && row[1] == "CLIENT_LIST" {
				clientHeader = openVPNHeaderIndex(row[2:])
			}
			inV1ClientList = false
			continue
		case "CLIENT_LIST":
			if len(row) < 2 {
				continue
			}
			commonName, rx, tx, ok := openVPNStatusClientFromRow(row[1:], clientHeader)
			if ok {
				addOpenVPNStatusClient(aggregates, commonName, rx, tx)
			}
			continue
		case "ROUTING TABLE", "GLOBAL STATS", "END":
			inV1ClientList = false
			continue
		}
		if row[0] == "Common Name" {
			v1ClientHeader = openVPNHeaderIndex(row)
			inV1ClientList = true
			continue
		}
		if inV1ClientList {
			commonName, rx, tx, ok := openVPNStatusClientFromRow(row, v1ClientHeader)
			if ok {
				addOpenVPNStatusClient(aggregates, commonName, rx, tx)
			}
		}
	}
	out := make([]openVPNStatusClient, 0, len(aggregates))
	for _, client := range aggregates {
		out = append(out, client)
	}
	return out
}

func openVPNHeaderIndex(header []string) map[string]int {
	out := map[string]int{}
	for idx, value := range header {
		out[strings.ToLower(strings.TrimSpace(value))] = idx
	}
	return out
}

func openVPNStatusClientFromRow(row []string, header map[string]int) (string, int64, int64, bool) {
	commonName, ok := openVPNRowField(row, header, "common name")
	if !ok || commonName == "" {
		return "", 0, 0, false
	}
	rxText, rxOK := openVPNRowField(row, header, "bytes received")
	txText, txOK := openVPNRowField(row, header, "bytes sent")
	if !rxOK || !txOK {
		return "", 0, 0, false
	}
	rx, rxErr := strconv.ParseInt(strings.TrimSpace(rxText), 10, 64)
	tx, txErr := strconv.ParseInt(strings.TrimSpace(txText), 10, 64)
	if rxErr != nil || txErr != nil {
		return "", 0, 0, false
	}
	return commonName, rx, tx, true
}

func openVPNRowField(row []string, header map[string]int, field string) (string, bool) {
	idx, ok := header[field]
	if !ok || idx < 0 || idx >= len(row) {
		return "", false
	}
	return strings.TrimSpace(row[idx]), true
}

func addOpenVPNStatusClient(aggregates map[string]openVPNStatusClient, commonName string, rx, tx int64) {
	commonName = strings.TrimSpace(commonName)
	if commonName == "" {
		return
	}
	current := aggregates[commonName]
	current.CommonName = commonName
	current.RxBytes += rx
	current.TxBytes += tx
	aggregates[commonName] = current
}

func (c *client) xrayTrafficAccountingSamples(readings []xrayTrafficReading, bucketStart, bucketEnd time.Time) []trafficAccountingSample {
	if c.xrayTrafficCounterState == nil {
		c.xrayTrafficCounterState = map[string]int64{}
	}
	aggregates := map[string]xrayTrafficAggregate{}
	for _, reading := range readings {
		key := xrayTrafficReadingKey(reading)
		previous, ok := c.xrayTrafficCounterState[key]
		if !ok || reading.Value < previous {
			continue
		}
		delta := reading.Value - previous
		if delta <= 0 {
			continue
		}
		aggregateKey := reading.InstanceID + "\x1f" + reading.User
		aggregate := aggregates[aggregateKey]
		aggregate.InstanceID = reading.InstanceID
		aggregate.InstanceSlug = reading.InstanceSlug
		aggregate.Endpoint = reading.Endpoint
		aggregate.User = reading.User
		if reading.Direction == "uplink" {
			aggregate.RxBytes += delta
		} else {
			aggregate.TxBytes += delta
		}
		aggregates[aggregateKey] = aggregate
	}
	samples := make([]trafficAccountingSample, 0, len(aggregates))
	observedAt := bucketEnd
	for _, aggregate := range aggregates {
		if aggregate.RxBytes == 0 && aggregate.TxBytes == 0 {
			continue
		}
		samples = append(samples, trafficAccountingSample{
			SampleKey:   fmt.Sprintf("xray:%s:%s:%d", aggregate.InstanceID, aggregate.User, bucketEnd.Unix()),
			InstanceID:  aggregate.InstanceID,
			Source:      "agent",
			Protocol:    "vless",
			Direction:   "bidirectional",
			BucketStart: &bucketStart,
			BucketEnd:   &bucketEnd,
			RxBytes:     aggregate.RxBytes,
			TxBytes:     aggregate.TxBytes,
			Metadata: map[string]any{
				"collector":     xrayStatsCollectorName,
				"xray_user":     aggregate.User,
				"client_user":   aggregate.User,
				"instance_slug": aggregate.InstanceSlug,
				"stats_api":     aggregate.Endpoint,
			},
			ObservedAt: &observedAt,
		})
	}
	return samples
}

func (c *client) commitXrayTrafficReadings(readings []xrayTrafficReading) {
	if c.xrayTrafficCounterState == nil {
		c.xrayTrafficCounterState = map[string]int64{}
	}
	for _, reading := range readings {
		c.xrayTrafficCounterState[xrayTrafficReadingKey(reading)] = reading.Value
	}
}

func xrayTrafficReadingKey(reading xrayTrafficReading) string {
	return reading.InstanceID + "\x1f" + reading.User + "\x1f" + reading.Direction
}

func (c *client) wireGuardTrafficAccountingSamples(readings []wireGuardTrafficReading, bucketStart, bucketEnd time.Time) []trafficAccountingSample {
	if c.wireGuardTrafficCounterState == nil {
		c.wireGuardTrafficCounterState = map[string]int64{}
	}
	samples := make([]trafficAccountingSample, 0, len(readings))
	observedAt := bucketEnd
	for _, reading := range readings {
		rxKey := wireGuardTrafficReadingKey(reading, "rx")
		txKey := wireGuardTrafficReadingKey(reading, "tx")
		previousRx, rxOK := c.wireGuardTrafficCounterState[rxKey]
		previousTx, txOK := c.wireGuardTrafficCounterState[txKey]
		if !rxOK || !txOK || reading.RxBytes < previousRx || reading.TxBytes < previousTx {
			continue
		}
		deltaRx := reading.RxBytes - previousRx
		deltaTx := reading.TxBytes - previousTx
		if deltaRx <= 0 && deltaTx <= 0 {
			continue
		}
		metadata := map[string]any{
			"collector":                   wireGuardTransferCollectorName,
			"wireguard_client_public_key": reading.PublicKey,
			"wireguard_client_address":    reading.ClientAddress,
			"instance_slug":               reading.InstanceSlug,
			"interface":                   reading.InterfaceName,
		}
		if reading.User != "" {
			metadata["client_user"] = reading.User
		}
		if reading.ServiceAccessID != "" {
			metadata["service_access_id"] = reading.ServiceAccessID
		}
		samples = append(samples, trafficAccountingSample{
			SampleKey:       fmt.Sprintf("wireguard:%s:%s:%d", reading.InstanceID, reading.PublicKey, bucketEnd.Unix()),
			InstanceID:      reading.InstanceID,
			ServiceAccessID: reading.ServiceAccessID,
			Source:          "agent",
			Protocol:        "wireguard",
			Direction:       "bidirectional",
			BucketStart:     &bucketStart,
			BucketEnd:       &bucketEnd,
			RxBytes:         deltaRx,
			TxBytes:         deltaTx,
			Metadata:        metadata,
			ObservedAt:      &observedAt,
		})
	}
	return samples
}

func (c *client) commitWireGuardTrafficReadings(readings []wireGuardTrafficReading) {
	if c.wireGuardTrafficCounterState == nil {
		c.wireGuardTrafficCounterState = map[string]int64{}
	}
	for _, reading := range readings {
		c.wireGuardTrafficCounterState[wireGuardTrafficReadingKey(reading, "rx")] = reading.RxBytes
		c.wireGuardTrafficCounterState[wireGuardTrafficReadingKey(reading, "tx")] = reading.TxBytes
	}
}

func wireGuardTrafficReadingKey(reading wireGuardTrafficReading, direction string) string {
	return reading.InstanceID + "\x1f" + reading.PublicKey + "\x1f" + direction
}

func (c *client) openVPNTrafficAccountingSamples(readings []openVPNTrafficReading, bucketStart, bucketEnd time.Time) []trafficAccountingSample {
	if c.openVPNTrafficCounterState == nil {
		c.openVPNTrafficCounterState = map[string]int64{}
	}
	samples := make([]trafficAccountingSample, 0, len(readings))
	observedAt := bucketEnd
	for _, reading := range readings {
		rxKey := openVPNTrafficReadingKey(reading, "rx")
		txKey := openVPNTrafficReadingKey(reading, "tx")
		previousRx, rxOK := c.openVPNTrafficCounterState[rxKey]
		previousTx, txOK := c.openVPNTrafficCounterState[txKey]
		if !rxOK || !txOK || reading.RxBytes < previousRx || reading.TxBytes < previousTx {
			continue
		}
		deltaRx := reading.RxBytes - previousRx
		deltaTx := reading.TxBytes - previousTx
		if deltaRx <= 0 && deltaTx <= 0 {
			continue
		}
		samples = append(samples, trafficAccountingSample{
			SampleKey:   fmt.Sprintf("openvpn:%s:%s:%d", reading.InstanceID, reading.CommonName, bucketEnd.Unix()),
			InstanceID:  reading.InstanceID,
			Source:      "agent",
			Protocol:    "openvpn",
			Direction:   "bidirectional",
			BucketStart: &bucketStart,
			BucketEnd:   &bucketEnd,
			RxBytes:     deltaRx,
			TxBytes:     deltaTx,
			Metadata: map[string]any{
				"collector":                  openVPNStatusCollectorName,
				"openvpn_client_common_name": reading.CommonName,
				"client_user":                reading.CommonName,
				"instance_slug":              reading.InstanceSlug,
				"status_path":                reading.StatusPath,
			},
			ObservedAt: &observedAt,
		})
	}
	return samples
}

func (c *client) commitOpenVPNTrafficReadings(readings []openVPNTrafficReading) {
	if c.openVPNTrafficCounterState == nil {
		c.openVPNTrafficCounterState = map[string]int64{}
	}
	for _, reading := range readings {
		c.openVPNTrafficCounterState[openVPNTrafficReadingKey(reading, "rx")] = reading.RxBytes
		c.openVPNTrafficCounterState[openVPNTrafficReadingKey(reading, "tx")] = reading.TxBytes
	}
}

func openVPNTrafficReadingKey(reading openVPNTrafficReading, direction string) string {
	return reading.InstanceID + "\x1f" + reading.CommonName + "\x1f" + direction
}

func normalizeServiceCode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "xray", "xray_core", "xray-core":
		return "xray-core"
	case "wireguard", "wg", "wg-quick":
		return "wireguard"
	case "openvpn", "openvpn-server":
		return "openvpn"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func joinTrafficAccountingErrors(errs ...error) error {
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			parts = append(parts, err.Error())
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return errors.New(strings.Join(parts, "; "))
}

func intFromConfigAny(value any) int {
	switch x := value.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		return 0
	}
}

func int64FromAny(value any) int64 {
	switch x := value.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	default:
		return 0
	}
}
