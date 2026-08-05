package externalegress

import "testing"

func TestParseVLESSRealityURL(t *testing.T) {
	raw := "vless://123e4567-e89b-42d3-a456-426614174000@vpn.example.com:443?type=tcp&security=reality&sni=www.example.com&fp=chrome&pbk=0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcde&sid=0123456789abcdef&flow=xtls-rprx-vision-udp443&spx=%2Fprovider&headerType=none"
	preview, err := ParseVLESS([]byte(raw))
	if err != nil {
		t.Fatalf("ParseVLESS returned error: %v", err)
	}
	if preview.EndpointHost != "vpn.example.com" || preview.EndpointPort != 443 || preview.Security != "reality" || !preview.HasCredential || preview.SpiderX != "/provider" {
		t.Fatalf("unexpected preview: %#v", preview)
	}
}

func TestParseVLESSRealityAllowsProviderWithoutShortID(t *testing.T) {
	raw := "vless://123e4567-e89b-42d3-a456-426614174000@vpn.example.com:443?type=grpc&security=reality&sni=www.example.com&fp=chrome&pbk=0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcde&serviceName=provider"
	if _, err := ParseVLESS([]byte(raw)); err != nil {
		t.Fatalf("ParseVLESS rejected an optional empty REALITY short ID: %v", err)
	}
}

func TestParseVLESSWebSocketTLSJSON(t *testing.T) {
	raw := `{"address":"vpn.example.com","port":443,"id":"123e4567-e89b-42d3-a456-426614174000","network":"ws","security":"tls","server_name":"cdn.example.com","path":"/sync","ws_host":"cdn.example.com","alpn":["h2","http/1.1"]}`
	preview, err := ParseVLESS([]byte(raw))
	if err != nil {
		t.Fatalf("ParseVLESS returned error: %v", err)
	}
	if preview.Transport != "ws" || preview.Path != "/sync" || preview.ServerName != "cdn.example.com" || len(preview.ALPN) != 2 {
		t.Fatalf("unexpected preview: %#v", preview)
	}
}

func TestParseVLESSWebSocketDefaultPathWarning(t *testing.T) {
	raw := `{"address":"vpn.example.com","port":443,"id":"123e4567-e89b-42d3-a456-426614174000","network":"ws","security":"tls","server_name":"cdn.example.com","alpn":["h2"," "]}`
	preview, err := ParseVLESS([]byte(raw))
	if err != nil {
		t.Fatalf("ParseVLESS returned error: %v", err)
	}
	if len(preview.Warnings) != 1 || preview.Warnings[0] != "WebSocket path defaults to /" {
		t.Fatalf("unexpected warnings: %#v", preview.Warnings)
	}
	if len(preview.ALPN) != 1 || preview.ALPN[0] != "h2" {
		t.Fatalf("unexpected ALPN list: %#v", preview.ALPN)
	}
}

func TestParseVLESSTLSDefaultsServerNameToDNSAddress(t *testing.T) {
	raw := "vless://123e4567-e89b-42d3-a456-426614174000@vpn.example.com:443?type=tcp&security=tls"
	preview, err := ParseVLESS([]byte(raw))
	if err != nil {
		t.Fatalf("ParseVLESS returned error: %v", err)
	}
	if preview.ServerName != "vpn.example.com" {
		t.Fatalf("server name = %q", preview.ServerName)
	}
}

func TestParseVLESSRejectsInsecureAndIncompleteReality(t *testing.T) {
	for _, raw := range []string{
		"vless://123e4567-e89b-42d3-a456-426614174000@vpn.example.com:443?type=tcp&security=none",
		"vless://123e4567-e89b-42d3-a456-426614174000@vpn.example.com:443?type=ws&security=reality&sni=www.example.com&pbk=key&sid=id",
		"vless://123e4567-e89b-42d3-a456-426614174000@vpn.example.com:443?type=tcp&security=tls&sni=www.example.com&allowInsecure=1",
		"vless://123e4567-e89b-42d3-a456-426614174000@vpn.example.com:443?type=tcp&security=tls&sni=www.example.com&postUp=run",
		`{"address":"vpn.example.com","port":443,"id":"123e4567-e89b-42d3-a456-426614174000","security":"tls","server_name":"vpn.example.com","unknown":"ignored"}`,
		`{"address":"vpn.example.com","port":"443.5","id":"123e4567-e89b-42d3-a456-426614174000","security":"tls","server_name":"vpn.example.com"}`,
	} {
		if _, err := ParseVLESS([]byte(raw)); err == nil {
			t.Fatalf("expected unsafe VLESS profile to be rejected: %s", raw)
		}
	}
}
