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

func TestParseWireGuardTransferOutput(t *testing.T) {
	records := parseWireGuardTransferOutput("client-public-key\t128\t512\ninvalid line\n")
	if len(records) != 1 {
		t.Fatalf("records = %#v, want one", records)
	}
	if records[0].PublicKey != "client-public-key" || records[0].RxBytes != 128 || records[0].TxBytes != 512 {
		t.Fatalf("record = %#v, want parsed wireguard transfer counters", records[0])
	}
}

func TestParseWireGuardPeerMetadata(t *testing.T) {
	peers := parseWireGuardPeerMetadata(`
[Interface]
Address = 10.66.0.1/24

# megavpn-service-access-id = 3d7f9a79-8738-41fa-9322-58b8bc12b10e
# megavpn-client = nlgate.999-iphone
[Peer]
PublicKey = client-public-key
AllowedIPs = 10.66.0.2/32, fd00::2/128
`)
	peer, ok := peers["client-public-key"]
	if !ok {
		t.Fatalf("peers = %#v, want client-public-key", peers)
	}
	if peer.ServiceAccessID != "3d7f9a79-8738-41fa-9322-58b8bc12b10e" || peer.User != "nlgate.999-iphone" {
		t.Fatalf("peer attribution = %#v", peer)
	}
	if got := firstWireGuardAllowedIP(peer.AllowedIPs); got != "10.66.0.2/32" {
		t.Fatalf("first allowed ip = %q, want 10.66.0.2/32", got)
	}
}

func TestWireGuardTrafficAccountingSamplesUseDeltas(t *testing.T) {
	c := &client{wireGuardTrafficCounterState: map[string]int64{
		"inst-1\x1fclient-public-key\x1frx": 100,
		"inst-1\x1fclient-public-key\x1ftx": 200,
	}}
	start := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	samples := c.wireGuardTrafficAccountingSamples([]wireGuardTrafficReading{
		{InstanceID: "inst-1", InstanceSlug: "edge-wg", InterfaceName: "wg0", PublicKey: "client-public-key", ClientAddress: "10.66.0.2/32", User: "nlgate.999-iphone", RxBytes: 150, TxBytes: 260},
	}, start, end)
	if len(samples) != 1 {
		t.Fatalf("samples = %#v, want one", samples)
	}
	if samples[0].RxBytes != 50 || samples[0].TxBytes != 60 {
		t.Fatalf("sample rx/tx = %d/%d, want 50/60", samples[0].RxBytes, samples[0].TxBytes)
	}
	if got := stringify(samples[0].Metadata["wireguard_client_address"]); got != "10.66.0.2/32" {
		t.Fatalf("wireguard address metadata = %q", got)
	}
}

func TestParseOpenVPNStatusClientsVersion2(t *testing.T) {
	clients := parseOpenVPNStatusClients(`TITLE,OpenVPN 2.6 Status
TIME,2026-07-06 12:00:00,1783339200
HEADER,CLIENT_LIST,Common Name,Real Address,Virtual Address,Virtual IPv6 Address,Bytes Received,Bytes Sent,Connected Since,Connected Since (time_t),Username,Client ID,Peer ID,Data Channel Cipher
CLIENT_LIST,nlgate.999-iphone,192.0.2.10:55123,10.8.0.2,,128,512,2026-07-06 11:59:00,1783339140,UNDEF,0,0,AES-256-GCM
END
`)
	if len(clients) != 1 {
		t.Fatalf("clients = %#v, want one", clients)
	}
	if clients[0].CommonName != "nlgate.999-iphone" || clients[0].RxBytes != 128 || clients[0].TxBytes != 512 {
		t.Fatalf("client = %#v, want parsed counters", clients[0])
	}
}

func TestParseOpenVPNStatusClientsVersion1AggregatesDuplicateCommonName(t *testing.T) {
	clients := parseOpenVPNStatusClients(`OpenVPN CLIENT LIST
Updated,2026-07-06 12:00:00
Common Name,Real Address,Bytes Received,Bytes Sent,Connected Since
nlgate.999-iphone,192.0.2.10:55123,100,200,2026-07-06 11:59:00
nlgate.999-iphone,192.0.2.11:55124,25,75,2026-07-06 11:59:30
ROUTING TABLE
`)
	if len(clients) != 1 {
		t.Fatalf("clients = %#v, want one aggregate", clients)
	}
	if clients[0].CommonName != "nlgate.999-iphone" || clients[0].RxBytes != 125 || clients[0].TxBytes != 275 {
		t.Fatalf("client aggregate = %#v", clients[0])
	}
}

func TestOpenVPNTrafficAccountingSamplesUseDeltas(t *testing.T) {
	c := &client{openVPNTrafficCounterState: map[string]int64{
		"inst-1\x1fnlgate.999-iphone\x1frx": 1000,
		"inst-1\x1fnlgate.999-iphone\x1ftx": 2000,
	}}
	start := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	samples := c.openVPNTrafficAccountingSamples([]openVPNTrafficReading{
		{InstanceID: "inst-1", InstanceSlug: "edge-ovpn", StatusPath: "/etc/openvpn/server/edge/status.log", CommonName: "nlgate.999-iphone", RxBytes: 1400, TxBytes: 2600},
	}, start, end)
	if len(samples) != 1 {
		t.Fatalf("samples = %#v, want one", samples)
	}
	if samples[0].RxBytes != 400 || samples[0].TxBytes != 600 {
		t.Fatalf("sample rx/tx = %d/%d, want 400/600", samples[0].RxBytes, samples[0].TxBytes)
	}
	if got := stringify(samples[0].Metadata["openvpn_client_common_name"]); got != "nlgate.999-iphone" {
		t.Fatalf("openvpn common-name metadata = %q", got)
	}
}
