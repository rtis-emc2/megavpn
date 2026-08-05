package http

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestDefaultServicePacksAreCreateReady(t *testing.T) {
	packs := servicePackDefinitions()
	if len(packs) == 0 {
		t.Fatal("expected default service packs")
	}
	seen := map[string]bool{}
	for _, pack := range packs {
		if strings.TrimSpace(pack.Key) == "" {
			t.Fatal("service pack key is required")
		}
		if seen[pack.Key] {
			t.Fatalf("duplicate service pack key %q", pack.Key)
		}
		seen[pack.Key] = true
		if strings.TrimSpace(pack.Label) == "" {
			t.Fatalf("service pack %q label is required", pack.Key)
		}
		if strings.TrimSpace(pack.BaseNameTemplate) == "" {
			t.Fatalf("service pack %q base_name_template is required", pack.Key)
		}
		if len(pack.Components) == 0 {
			t.Fatalf("service pack %q must define components", pack.Key)
		}
		for idx, component := range pack.Components {
			if strings.TrimSpace(component.ServiceCode) == "" {
				t.Fatalf("service pack %q component %d service_code is required", pack.Key, idx)
			}
			if strings.TrimSpace(component.NameSuffix) == "" {
				t.Fatalf("service pack %q component %d name_suffix is required", pack.Key, idx)
			}
			if strings.TrimSpace(component.SlugSuffix) == "" {
				t.Fatalf("service pack %q component %d slug_suffix is required", pack.Key, idx)
			}
			if component.Spec == nil {
				t.Fatalf("service pack %q component %d spec is required", pack.Key, idx)
			}
		}
	}
}

func TestDefaultXrayServicePacksEnableTrafficAccounting(t *testing.T) {
	for _, pack := range servicePackDefinitions() {
		for _, component := range pack.Components {
			if component.ServiceCode != "xray-core" {
				continue
			}
			if component.Spec["traffic_accounting_enabled"] != true {
				t.Fatalf("service pack %q component %q must enable Xray traffic accounting", pack.Key, component.Label)
			}
		}
	}
}

func TestDefaultAccessSuiteHasExpectedComponentsWithoutInlineSecrets(t *testing.T) {
	pack, found := findServicePack("default_access_suite")
	if !found {
		t.Fatal("default_access_suite service pack is required")
	}
	expected := []struct {
		serviceCode string
		presetKey   string
		port        int
	}{
		{"xray-core", "reality_tcp", 443},
		{"openvpn", "tcp_11994", 11994},
		{"xray-core", "reality_tcp", 8443},
		{"openvpn", "udp_1194", 1194},
		{"shadowsocks", "chacha_full", 8388},
		{"wireguard", "roadwarrior", 51820},
	}
	if len(pack.Components) != len(expected) {
		t.Fatalf("component count = %d, want %d", len(pack.Components), len(expected))
	}
	for idx, want := range expected {
		component := pack.Components[idx]
		if component.ServiceCode != want.serviceCode || component.PresetKey != want.presetKey || component.EndpointPort != want.port {
			t.Fatalf("component %d = %s/%s/%d, want %s/%s/%d", idx, component.ServiceCode, component.PresetKey, component.EndpointPort, want.serviceCode, want.presetKey, want.port)
		}
		for _, secretKey := range []string{
			"password",
			"server_password",
			"server_private_key",
			"reality_private_key",
			"short_id",
			"short_ids",
			"psk",
			"private_key",
		} {
			if _, exists := component.Spec[secretKey]; exists {
				t.Fatalf("component %d must not include inline secret key %q", idx, secretKey)
			}
		}
	}
}

func TestSelectServicePackComponentsHTTPDefaultsToFullPack(t *testing.T) {
	pack, found := findServicePack("default_access_suite")
	if !found {
		t.Fatal("default_access_suite service pack is required")
	}
	selected, err := selectServicePackComponentsHTTP(pack, createServicePackRequest{})
	if err != nil {
		t.Fatalf("selectServicePackComponentsHTTP() error = %v", err)
	}
	components := servicePackSelectedComponentListHTTP(selected)
	if len(components) != len(pack.Components) {
		t.Fatalf("selected component count = %d, want %d", len(components), len(pack.Components))
	}
	components[0].Spec["service_profile"] = "mutated"
	if pack.Components[0].Spec["service_profile"] == "mutated" {
		t.Fatal("selected components must not mutate the service pack template")
	}
}

func TestSelectServicePackComponentsHTTPFiltersAndOverrides(t *testing.T) {
	pack, found := findServicePack("default_access_suite")
	if !found {
		t.Fatal("default_access_suite service pack is required")
	}
	first := 0
	openVPN := 3
	selected, err := selectServicePackComponentsHTTP(pack, createServicePackRequest{
		Components: []createServicePackComponentRequest{
			{Index: &first, EndpointPort: 7443},
			{Index: &openVPN, EndpointPort: 11950, OpenVPNPKIProfile: "edge-ca"},
		},
	})
	if err != nil {
		t.Fatalf("selectServicePackComponentsHTTP() error = %v", err)
	}
	components := servicePackSelectedComponentListHTTP(selected)
	if len(components) != 2 {
		t.Fatalf("selected component count = %d, want 2", len(components))
	}
	if components[0].Label != "VLESS TCP Edge" || components[0].EndpointPort != 7443 {
		t.Fatalf("first selected component = %#v", components[0])
	}
	if components[1].Label != "OpenVPN UDP" || components[1].EndpointPort != 11950 {
		t.Fatalf("second selected component = %#v", components[1])
	}
	if selected[1].OpenVPNPKIProfile != "edge-ca" {
		t.Fatalf("OpenVPNPKIProfile = %q, want edge-ca", selected[1].OpenVPNPKIProfile)
	}
	missing := missingServicePackRuntimeCapabilities(components, []domain.NodeCapability{})
	if len(missing) != 2 || missing[0] != "xray-core" || missing[1] != "openvpn" {
		t.Fatalf("missing runtime capabilities = %#v, want selected runtimes only", missing)
	}
}

func TestSelectServicePackComponentsHTTPRejectsInvalidSelections(t *testing.T) {
	pack, found := findServicePack("default_access_suite")
	if !found {
		t.Fatal("default_access_suite service pack is required")
	}
	zero := 0
	one := 1
	tooHigh := len(pack.Components)
	cases := []struct {
		name string
		req  createServicePackRequest
	}{
		{name: "empty explicit selection", req: createServicePackRequest{Components: []createServicePackComponentRequest{}}},
		{name: "missing index", req: createServicePackRequest{Components: []createServicePackComponentRequest{{EndpointPort: 443}}}},
		{name: "out of range", req: createServicePackRequest{Components: []createServicePackComponentRequest{{Index: &tooHigh}}}},
		{name: "duplicate index", req: createServicePackRequest{Components: []createServicePackComponentRequest{{Index: &zero}, {Index: &zero}}}},
		{name: "bad port", req: createServicePackRequest{Components: []createServicePackComponentRequest{{Index: &zero, EndpointPort: 70000}}}},
		{name: "openvpn profile on xray", req: createServicePackRequest{Components: []createServicePackComponentRequest{{Index: &zero, OpenVPNPKIProfile: "edge-ca"}}}},
		{name: "openvpn profile on openvpn companion accepted", req: createServicePackRequest{Components: []createServicePackComponentRequest{{Index: &one, OpenVPNPKIProfile: "edge-ca"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := selectServicePackComponentsHTTP(pack, tc.req)
			if tc.name == "openvpn profile on openvpn companion accepted" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestFindExistingServicePackComponentInstancePrefersCanonicalPackInstance(t *testing.T) {
	pack, found := findServicePack("xray_nginx_http_edge")
	if !found {
		t.Fatal("xray_nginx_http_edge service pack is required")
	}
	component := pack.Components[0]
	baseName, baseSlug := servicePackComponentBaseIdentity("portal-edge", component)
	now := time.Now().UTC()
	instances := []domain.Instance{
		{
			ID:           "duplicate",
			NodeID:       "node-1",
			ServiceCode:  "xray-core",
			Name:         baseName + " 2",
			Slug:         baseSlug + "-2",
			Status:       "provisioning",
			EndpointHost: "portal.nlgate.ru",
			EndpointPort: 7080,
			CreatedAt:    now.Add(-2 * time.Minute),
		},
		{
			ID:           "canonical",
			NodeID:       "node-1",
			ServiceCode:  "xray-core",
			Name:         baseName,
			Slug:         baseSlug,
			Status:       "provisioning",
			EndpointHost: "portal.nlgate.ru",
			EndpointPort: 7080,
			CreatedAt:    now.Add(-time.Minute),
		},
		{
			ID:           "other-host",
			NodeID:       "node-1",
			ServiceCode:  "xray-core",
			Name:         baseName,
			Slug:         baseSlug,
			Status:       "provisioning",
			EndpointHost: "other.nlgate.ru",
			EndpointPort: 7080,
			CreatedAt:    now.Add(-3 * time.Minute),
		},
	}

	got, ok := findExistingServicePackComponentInstance(instances, "node-1", component, baseName, baseSlug, "portal.nlgate.ru")
	if !ok {
		t.Fatal("expected existing service pack component instance")
	}
	if got.ID != "canonical" {
		t.Fatalf("existing instance = %q, want canonical", got.ID)
	}

	if _, ok := findExistingServicePackComponentInstance(instances, "node-1", component, baseName, baseSlug, "new.nlgate.ru"); ok {
		t.Fatal("different endpoint host must not match existing component")
	}
}

func TestDefaultOpenVPNPacksUseFullTunnelServerPush(t *testing.T) {
	for _, pack := range servicePackDefinitions() {
		for _, component := range pack.Components {
			if component.ServiceCode != "openvpn" {
				continue
			}
			lines := stringSliceFromAny(component.Spec["server_extra_lines"])
			for _, want := range openVPNFullTunnelServerExtraLines() {
				if !containsString(lines, want) {
					t.Fatalf("pack %s component %s server_extra_lines = %#v, missing %q", pack.Key, component.Label, lines, want)
				}
			}
		}
	}
}

func TestDefaultWebSocketCamouflagePackUsesLoopbackBackendAndFallbackPlaceholders(t *testing.T) {
	pack, found := findServicePack("xray_nginx_http_edge")
	if !found {
		t.Fatal("xray_nginx_http_edge service pack is required")
	}
	if !servicePackUsesTrafficCamouflage(pack) {
		t.Fatal("xray_nginx_http_edge must be detected as traffic camouflage pack")
	}
	var xraySpec, nginxSpec map[string]any
	for _, component := range pack.Components {
		switch strings.TrimSpace(stringifyHTTP(component.Spec["service_profile"])) {
		case "nginx_ws_backend":
			xraySpec = component.Spec
		case "ws_camouflage_edge":
			nginxSpec = component.Spec
		}
	}
	if xraySpec == nil || nginxSpec == nil {
		t.Fatalf("camouflage pack components missing: xray=%v nginx=%v", xraySpec != nil, nginxSpec != nil)
	}
	if got := stringifyHTTP(xraySpec["listen"]); got != "127.0.0.1" {
		t.Fatalf("xray backend listen = %q, want loopback", got)
	}
	if got := stringifyHTTP(xraySpec["path"]); got != "{{camouflage_path}}" {
		t.Fatalf("xray ws path = %q, want camouflage placeholder", got)
	}
	if got := stringifyHTTP(nginxSpec["location_path"]); got != "{{camouflage_path}}" {
		t.Fatalf("nginx location_path = %q, want camouflage placeholder", got)
	}
	if got := stringifyHTTP(nginxSpec["fallback_upstream_url"]); got != "{{fallback_upstream_url}}" {
		t.Fatalf("nginx fallback upstream = %q, want fallback placeholder", got)
	}
	if enabled, _ := nginxSpec["websocket_upgrade"].(bool); !enabled {
		t.Fatalf("nginx websocket_upgrade must be enabled for camouflage edge: %#v", nginxSpec)
	}
	if forwardClientIP, ok := nginxSpec["forward_client_ip"].(bool); !ok || forwardClientIP {
		t.Fatalf("nginx camouflage edge must keep fallback privacy by default: %#v", nginxSpec)
	}
	if _, ok := nginxSpec["location_extra_lines"]; ok {
		t.Fatalf("default camouflage pack must use structured websocket_upgrade instead of raw location_extra_lines: %#v", nginxSpec["location_extra_lines"])
	}
}

func TestNormalizeServicePackCamouflageRequestDerivesFallbackHostAndSNI(t *testing.T) {
	pack, found := findServicePack("xray_nginx_http_edge")
	if !found {
		t.Fatal("xray_nginx_http_edge service pack is required")
	}
	req := createServicePackRequest{FallbackUpstreamURL: "https://target.example.com/app"}
	if err := normalizeServicePackCamouflageRequestHTTP(&req, pack); err != nil {
		t.Fatalf("normalizeServicePackCamouflageRequestHTTP() error = %v", err)
	}
	if req.CamouflagePath != "/assets/rtis-sync" {
		t.Fatalf("CamouflagePath = %q, want /assets/rtis-sync", req.CamouflagePath)
	}
	if req.FallbackHostHeader != "target.example.com" {
		t.Fatalf("FallbackHostHeader = %q, want target.example.com", req.FallbackHostHeader)
	}
	if req.FallbackSNI != "target.example.com" {
		t.Fatalf("FallbackSNI = %q, want target.example.com", req.FallbackSNI)
	}
}

func TestNormalizeServicePackCamouflageRequestIgnoresPathWhenPackDoesNotTemplateIt(t *testing.T) {
	pack, found := findServicePack("xray_nginx_grpc_edge")
	if !found {
		t.Fatal("xray_nginx_grpc_edge service pack is required")
	}
	if !servicePackUsesTrafficCamouflage(pack) {
		t.Fatal("xray_nginx_grpc_edge must be detected as traffic camouflage pack")
	}
	if servicePackUsesConfigurableCamouflagePathHTTP(pack) {
		t.Fatal("xray_nginx_grpc_edge should keep its fixed gRPC location path")
	}
	req := createServicePackRequest{
		CamouflagePath:      "/ignored;unsafe",
		FallbackUpstreamURL: "https://target.example.com",
	}
	if err := normalizeServicePackCamouflageRequestHTTP(&req, pack); err != nil {
		t.Fatalf("normalizeServicePackCamouflageRequestHTTP() error = %v", err)
	}
	if req.CamouflagePath != "" {
		t.Fatalf("CamouflagePath = %q, want empty because grpc pack does not template it", req.CamouflagePath)
	}
}

func TestNormalizeServicePackCamouflageRequestRejectsUnsafeInputs(t *testing.T) {
	pack, found := findServicePack("xray_nginx_http_edge")
	if !found {
		t.Fatal("xray_nginx_http_edge service pack is required")
	}
	cases := []createServicePackRequest{
		{CamouflagePath: "/", FallbackUpstreamURL: "https://target.example.com"},
		{CamouflagePath: "/assets/ws;return", FallbackUpstreamURL: "https://target.example.com"},
		{CamouflagePath: "/assets/ws?token=1", FallbackUpstreamURL: "https://target.example.com"},
		{CamouflagePath: "/assets/ws", FallbackUpstreamURL: "ftp://target.example.com"},
		{CamouflagePath: "/assets/ws", FallbackUpstreamURL: "https://user:pass@target.example.com"},
		{CamouflagePath: "/assets/ws", FallbackUpstreamURL: "https://target.example.com/#fragment"},
	}
	for _, req := range cases {
		req := req
		t.Run(req.CamouflagePath+" "+req.FallbackUpstreamURL, func(t *testing.T) {
			if err := normalizeServicePackCamouflageRequestHTTP(&req, pack); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNormalizeServicePackCamouflageRequestRejectsFallbackLoopToEndpoint(t *testing.T) {
	pack, found := findServicePack("xray_nginx_http_edge")
	if !found {
		t.Fatal("xray_nginx_http_edge service pack is required")
	}
	cases := []createServicePackRequest{
		{EndpointHost: "enter.example.com", FallbackUpstreamURL: "https://enter.example.com"},
		{EndpointHost: "enter.example.com:443", FallbackUpstreamURL: "https://enter.example.com/site"},
		{EndpointHost: "enter.example.com.", FallbackUpstreamURL: "https://ENTER.EXAMPLE.COM"},
		{EndpointHost: "enter.example.com", FallbackUpstreamURL: "https://target.example.com", FallbackHostHeader: "enter.example.com"},
		{EndpointHost: "enter.example.com", FallbackUpstreamURL: "https://target.example.com", FallbackSNI: "enter.example.com"},
	}
	for _, req := range cases {
		req := req
		t.Run(req.FallbackUpstreamURL, func(t *testing.T) {
			if err := normalizeServicePackCamouflageRequestHTTP(&req, pack); err == nil {
				t.Fatal("expected fallback loop validation error")
			}
		})
	}
}

func TestServicePackInstanceIdentitySetAllocatesUniqueNameAndSlug(t *testing.T) {
	set := newServicePackInstanceIdentitySet([]domain.Instance{
		{NodeID: "node-1", Name: "edge-access-wireguard", Slug: "edge-access-wireguard", Status: "failed"},
		{NodeID: "node-2", Name: "edge-access-wireguard 2", Slug: "edge-access-wireguard-2", Status: "active"},
		{NodeID: "node-1", Name: "old-deleted", Slug: "old-deleted", Status: "deleted"},
	})

	name, slug := set.reserve("node-1", "edge-access-wireguard", "edge-access-wireguard")
	if name != "edge-access-wireguard 3" || slug != "edge-access-wireguard-3" {
		t.Fatalf("reserved identity = %q/%q, want suffix 3", name, slug)
	}
	name, slug = set.reserve("node-1", "edge-access-wireguard", "edge-access-wireguard")
	if name != "edge-access-wireguard 4" || slug != "edge-access-wireguard-4" {
		t.Fatalf("second reserved identity = %q/%q, want suffix 4", name, slug)
	}
}

func TestMissingServicePackRuntimeCapabilitiesDeduplicatesRuntime(t *testing.T) {
	components := []domain.ServicePackComponent{
		{ServiceCode: "xray-core"},
		{ServiceCode: "xray-core"},
		{ServiceCode: "openvpn"},
		{ServiceCode: "wireguard"},
	}
	capabilities := []domain.NodeCapability{
		{CapabilityCode: "openvpn", Status: "available"},
	}
	got := missingServicePackRuntimeCapabilities(components, capabilities)
	want := []string{"xray-core", "wireguard"}
	if len(got) != len(want) {
		t.Fatalf("missing runtime count = %d, want %d: %#v", len(got), len(want), got)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("missing runtime %d = %q, want %q; all=%#v", idx, got[idx], want[idx], got)
		}
	}
}

func TestMissingServicePackRuntimeCapabilitiesTreatsFailedRuntimeAsMissing(t *testing.T) {
	components := []domain.ServicePackComponent{
		{ServiceCode: "shadowsocks"},
		{ServiceCode: "openvpn"},
	}
	capabilities := []domain.NodeCapability{
		{CapabilityCode: "shadowsocks", Status: "failed"},
		{CapabilityCode: "openvpn", Status: "available"},
	}
	got := missingServicePackRuntimeCapabilities(components, capabilities)
	if len(got) != 1 || got[0] != "shadowsocks" {
		t.Fatalf("missing runtime capabilities = %#v, want [shadowsocks]", got)
	}
}

func TestServicePackTLSEdgeCertificateScope(t *testing.T) {
	cases := []struct {
		name      string
		component domain.ServicePackComponent
		want      bool
	}{
		{
			name:      "nginx always terminates edge TLS",
			component: domain.ServicePackComponent{ServiceCode: "nginx", Spec: map[string]any{}},
			want:      true,
		},
		{
			name:      "xray tls uses edge certificate",
			component: domain.ServicePackComponent{ServiceCode: "xray-core", Spec: map[string]any{"security": "tls"}},
			want:      true,
		},
		{
			name:      "xray reality does not use leaf TLS certificate",
			component: domain.ServicePackComponent{ServiceCode: "xray-core", Spec: map[string]any{"security": "reality"}},
			want:      false,
		},
		{
			name:      "openvpn uses service pki profile instead",
			component: domain.ServicePackComponent{ServiceCode: "openvpn", Spec: map[string]any{}},
			want:      false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := servicePackComponentUsesTLSEdgeCertificate(tc.component); got != tc.want {
				t.Fatalf("servicePackComponentUsesTLSEdgeCertificate() = %v, want %v", got, tc.want)
			}
			pack := domain.ServicePackDefinition{Components: []domain.ServicePackComponent{tc.component}}
			if got := servicePackUsesTLSEdgeCertificate(pack); got != tc.want {
				t.Fatalf("servicePackUsesTLSEdgeCertificate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNormalizeXrayEgressModeHTTP(t *testing.T) {
	cases := []struct {
		name         string
		mode         string
		egressNodeID string
		want         string
		wantErr      bool
	}{
		{name: "empty without node keeps pack default", mode: "", want: ""},
		{name: "empty with node becomes explicit egress node", mode: "", egressNodeID: "node-2", want: "egress_node"},
		{name: "auto", mode: "auto", want: "auto"},
		{name: "remote alias", mode: "remote_egress", egressNodeID: "node-2", want: "egress_node"},
		{name: "explicit egress node requires node id", mode: "egress_node", wantErr: true},
		{name: "local alias", mode: "direct", want: "local_breakout"},
		{name: "reject unknown", mode: "proxy", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeXrayEgressModeHTTP(tc.mode, tc.egressNodeID)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("mode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestApplyXrayEgressOverrideHTTP(t *testing.T) {
	xraySpec := map[string]any{"service_profile": "reality_tcp"}
	applyXrayEgressOverrideHTTP(
		domain.ServicePackComponent{ServiceCode: "xray-core"},
		xraySpec,
		"egress_node",
		"egress-node-id",
	)
	if xraySpec["egress_mode"] != "egress_node" {
		t.Fatalf("egress_mode = %v, want egress_node", xraySpec["egress_mode"])
	}
	if xraySpec["egress_node_id"] != "egress-node-id" {
		t.Fatalf("egress_node_id = %v, want egress-node-id", xraySpec["egress_node_id"])
	}
	nested, ok := xraySpec["xray_egress"].(map[string]any)
	if !ok {
		t.Fatalf("xray_egress = %#v, want object", xraySpec["xray_egress"])
	}
	if nested["mode"] != "egress_node" || nested["egress_node_id"] != "egress-node-id" {
		t.Fatalf("xray_egress = %#v", nested)
	}

	openVPNSpec := map[string]any{"service_profile": "udp_1194"}
	applyXrayEgressOverrideHTTP(
		domain.ServicePackComponent{ServiceCode: "openvpn"},
		openVPNSpec,
		"egress_node",
		"egress-node-id",
	)
	if _, exists := openVPNSpec["egress_mode"]; exists {
		t.Fatalf("non-xray spec must not receive egress override: %#v", openVPNSpec)
	}
}

func TestDefaultServicePackDefinitionsReturnsCopy(t *testing.T) {
	first := DefaultServicePackDefinitions()
	if len(first) == 0 || len(first[0].Components) == 0 {
		t.Fatal("expected default service pack definitions")
	}
	originalLabel := first[0].Label
	originalSpecValue := first[0].Components[0].Spec["service_profile"]
	first[0].Label = "mutated"
	first[0].Components[0].Spec["service_profile"] = "mutated"

	second := DefaultServicePackDefinitions()
	if second[0].Label != originalLabel {
		t.Fatalf("default pack label was mutated: %q", second[0].Label)
	}
	if second[0].Components[0].Spec["service_profile"] != originalSpecValue {
		t.Fatalf("default pack spec was mutated: %v", second[0].Components[0].Spec["service_profile"])
	}
}

func TestServicePackCatalogUnavailableDetection(t *testing.T) {
	if !isServicePackCatalogUnavailable(&pgconn.PgError{Code: "42P01", Message: "relation service_pack_templates does not exist"}) {
		t.Fatal("expected undefined table to be treated as catalog unavailable")
	}
	if !isServicePackCatalogUnavailable(&pgconn.PgError{Code: "42703", Message: "column display_order does not exist"}) {
		t.Fatal("expected undefined column to be treated as catalog unavailable")
	}
	if isServicePackCatalogUnavailable(&pgconn.PgError{Code: "23505", Message: "duplicate key"}) {
		t.Fatal("duplicate key must not be treated as catalog unavailable")
	}
}

func TestInterpolatePackSpecReplacesNestedPlaceholders(t *testing.T) {
	spec := map[string]any{
		"server_name": "{{endpoint_host}}",
		"nested": map[string]any{
			"url": "https://{{endpoint_host}}/{{component_slug}}",
		},
		"list": []any{"{{base_name}}", "static"},
	}
	out := interpolatePackSpec(spec, map[string]string{
		"endpoint_host":  "edge.example.com",
		"component_slug": "edge-xray",
		"base_name":      "edge",
	})
	if containsInterpolationToken(out) {
		t.Fatalf("interpolation left template tokens in %#v", out)
	}
	if out["server_name"] != "edge.example.com" {
		t.Fatalf("unexpected server_name %v", out["server_name"])
	}
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(stringifyHTTP(item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsInterpolationToken(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, "{{") || strings.Contains(typed, "}}")
	case map[string]any:
		for _, item := range typed {
			if containsInterpolationToken(item) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if containsInterpolationToken(item) {
				return true
			}
		}
	}
	return false
}
