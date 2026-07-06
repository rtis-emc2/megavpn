package main

import (
	"testing"
	"time"
)

func TestParseXrayStatsQueryOutputText(t *testing.T) {
	output := `
stat: <
  name: "user>>>nlgate.999-iphone>>>traffic>>>uplink"
  value: 128
>
stat: <
  name: "user>>>nlgate.999-iphone>>>traffic>>>downlink"
  value: 512
>
`
	records := parseXrayStatsQueryOutput(output)
	if len(records) != 2 {
		t.Fatalf("records = %#v, want 2", records)
	}
	user, direction, ok := parseXrayUserTrafficStatName(records[0].Name)
	if !ok || user != "nlgate.999-iphone" || direction != "uplink" || records[0].Value != 128 {
		t.Fatalf("first parsed record = user=%q direction=%q ok=%v value=%d", user, direction, ok, records[0].Value)
	}
}

func TestXrayTrafficAccountingSamplesUseDeltas(t *testing.T) {
	c := &client{xrayTrafficCounterState: map[string]int64{
		"inst-1\x1fclient-a\x1fuplink":   100,
		"inst-1\x1fclient-a\x1fdownlink": 200,
	}}
	start := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	samples := c.xrayTrafficAccountingSamples([]xrayTrafficReading{
		{InstanceID: "inst-1", InstanceSlug: "edge-xray", Endpoint: "127.0.0.1:17080", User: "client-a", Direction: "uplink", Value: 175},
		{InstanceID: "inst-1", InstanceSlug: "edge-xray", Endpoint: "127.0.0.1:17080", User: "client-a", Direction: "downlink", Value: 450},
	}, start, end)
	if len(samples) != 1 {
		t.Fatalf("samples = %#v, want one aggregate", samples)
	}
	if samples[0].RxBytes != 75 || samples[0].TxBytes != 250 {
		t.Fatalf("sample rx/tx = %d/%d, want 75/250", samples[0].RxBytes, samples[0].TxBytes)
	}
	if got := stringify(samples[0].Metadata["xray_user"]); got != "client-a" {
		t.Fatalf("metadata xray_user = %q, want client-a", got)
	}
}

func TestXrayTrafficAccountingSamplesBaselineFirstRead(t *testing.T) {
	c := &client{}
	start := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	samples := c.xrayTrafficAccountingSamples([]xrayTrafficReading{
		{InstanceID: "inst-1", User: "client-a", Direction: "uplink", Value: 175},
	}, start, end)
	if len(samples) != 0 {
		t.Fatalf("first read samples = %#v, want baseline-only", samples)
	}
	c.commitXrayTrafficReadings([]xrayTrafficReading{{InstanceID: "inst-1", User: "client-a", Direction: "uplink", Value: 175}})
	samples = c.xrayTrafficAccountingSamples([]xrayTrafficReading{
		{InstanceID: "inst-1", User: "client-a", Direction: "uplink", Value: 200},
	}, end, end.Add(time.Minute))
	if len(samples) != 1 || samples[0].RxBytes != 25 {
		t.Fatalf("second read samples = %#v, want rx delta 25", samples)
	}
}
