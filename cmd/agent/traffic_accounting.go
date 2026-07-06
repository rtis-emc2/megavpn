package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	xrayStatsCollectorName = "xray_stats"
	xrayStatsQueryPattern  = "user>>>"
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
	readings, err := collectXrayTrafficReadings(ctx, targets)
	if err != nil && len(readings) == 0 {
		c.lastTrafficReportAt = now
		return err
	}
	bucketStart := c.lastTrafficReportAt
	if bucketStart.IsZero() {
		bucketStart = now.Add(-c.trafficReportInterval)
	}
	samples := c.xrayTrafficAccountingSamples(readings, bucketStart, now)
	if len(samples) == 0 {
		c.commitXrayTrafficReadings(readings)
		c.lastTrafficReportAt = now
		return err
	}
	if submitErr := c.submitTrafficAccounting(ctx, nodeID, samples); submitErr != nil {
		return submitErr
	}
	c.commitXrayTrafficReadings(readings)
	c.lastTrafficReportAt = now
	return err
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

func normalizeServiceCode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "xray", "xray_core", "xray-core":
		return "xray-core"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
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
