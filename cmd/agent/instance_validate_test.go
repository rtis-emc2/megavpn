package main

import (
	"context"
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

func TestValidateRenderedConfigIgnoresUnknownService(t *testing.T) {
	t.Parallel()

	err := validateRenderedConfig(context.Background(), instanceJobPayload{ServiceCode: "custom-service"}, nil)
	if err != nil {
		t.Fatalf("validateRenderedConfig unknown service error = %v, want nil", err)
	}
}

func TestValidateConfigPresenceRejectsEmptyConfig(t *testing.T) {
	t.Parallel()

	err := validateRenderedConfig(context.Background(), instanceJobPayload{ServiceCode: driver.OpenVPN, Slug: "edge"}, []managedFileSpec{
		{Path: "/etc/openvpn/server/edge.conf", Content: " \n\t"},
	})
	if err == nil {
		t.Fatal("validateRenderedConfig openvpn empty config error = nil, want error")
	}
	if !strings.Contains(err.Error(), "config content is empty") {
		t.Fatalf("validateRenderedConfig error = %q, want empty content error", err.Error())
	}
}

func TestValidateShadowsocksConfigRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	err := validateRenderedConfig(context.Background(), instanceJobPayload{ServiceCode: driver.Shadowsocks, Slug: "ss-edge"}, []managedFileSpec{
		{Path: "/etc/shadowsocks-libev/ss-edge.json", Content: "{bad-json"},
	})
	if err == nil {
		t.Fatal("validateRenderedConfig shadowsocks invalid JSON error = nil, want error")
	}
	if !strings.Contains(err.Error(), "valid JSON") {
		t.Fatalf("validateRenderedConfig error = %q, want JSON validation error", err.Error())
	}
}

func TestValidateShadowsocksConfigAcceptsValidJSON(t *testing.T) {
	t.Parallel()

	err := validateRenderedConfig(context.Background(), instanceJobPayload{ServiceCode: "shadowsocks-libev", Slug: "ss-edge"}, []managedFileSpec{
		{Path: "/etc/shadowsocks-libev/ss-edge.json", Content: `{"server":"0.0.0.0","server_port":8388}`},
	})
	if err != nil {
		t.Fatalf("validateRenderedConfig valid shadowsocks config error = %v, want nil", err)
	}
}
