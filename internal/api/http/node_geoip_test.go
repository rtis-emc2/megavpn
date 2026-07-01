package http

import (
	"context"
	"testing"
)

func TestParseNodeGeoIPResponseCommonSchemas(t *testing.T) {
	tests := []struct {
		name        string
		payload     map[string]any
		wantCountry string
		wantASN     string
	}{
		{
			name: "ipapi",
			payload: map[string]any{
				"ip":           "203.0.113.10",
				"latitude":     52.37,
				"longitude":    4.90,
				"country_code": "NL",
				"country_name": "Netherlands",
				"city":         "Amsterdam",
				"org":          "Example Hosting",
				"asn":          "AS64500",
			},
			wantCountry: "Netherlands",
			wantASN:     "AS64500",
		},
		{
			name: "ipinfo",
			payload: map[string]any{
				"ip":      "198.51.100.10",
				"loc":     "40.7128,-74.0060",
				"country": "US",
				"city":    "New York",
				"org":     "AS64501 Example Network",
			},
			wantCountry: "",
			wantASN:     "AS64501",
		},
		{
			name: "nested_asn_company",
			payload: map[string]any{
				"ip":  "192.0.2.44",
				"lat": 50.45,
				"lon": 30.52,
				"country": map[string]any{
					"iso_code": "UA",
					"names": map[string]any{
						"en": "Ukraine",
					},
				},
				"asn": map[string]any{
					"asn":  64502,
					"name": "Example Datacenter",
				},
			},
			wantCountry: "Ukraine",
			wantASN:     "AS64502",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNodeGeoIPResponse("test", "192.0.2.1", tt.payload)
			if err != nil {
				t.Fatalf("parseNodeGeoIPResponse() error = %v", err)
			}
			if got.Status != "resolved" {
				t.Fatalf("Status = %q, want resolved", got.Status)
			}
			if got.CountryName != tt.wantCountry {
				t.Fatalf("CountryName = %q, want %q", got.CountryName, tt.wantCountry)
			}
			if got.ASN != tt.wantASN {
				t.Fatalf("ASN = %q, want %q", got.ASN, tt.wantASN)
			}
			if got.Latitude == nil || got.Longitude == nil {
				t.Fatalf("coordinates were not parsed")
			}
		})
	}
}

func TestPublicNodeIPRejectsPrivateAddresses(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.10.10.10", "[::1]:22"} {
		if _, err := publicNodeIP(context.Background(), address); err == nil {
			t.Fatalf("publicNodeIP(%q) succeeded for private address", address)
		}
	}
}

func TestNodeAddressHost(t *testing.T) {
	tests := map[string]string{
		"https://example.com:8443/path": "example.com",
		"203.0.113.10:22":               "203.0.113.10",
		"[2001:db8::1]:443":             "2001:db8::1",
	}
	for in, want := range tests {
		if got := nodeAddressHost(in); got != want {
			t.Fatalf("nodeAddressHost(%q) = %q, want %q", in, got, want)
		}
	}
}
