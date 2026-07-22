package externalegress

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type WireGuardPreview struct {
	Protocol        string   `json:"protocol"`
	EndpointHost    string   `json:"endpoint_host"`
	EndpointPort    int      `json:"endpoint_port"`
	Addresses       []string `json:"addresses"`
	AllowedIPs      []string `json:"allowed_ips"`
	HasPrivateKey   bool     `json:"has_private_key"`
	HasPresharedKey bool     `json:"has_preshared_key"`
	HasDefaultRoute bool     `json:"has_default_route"`
	RequiredSecrets []string `json:"required_secrets,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

func ParseWireGuard(raw []byte) (WireGuardPreview, error) {
	preview := WireGuardPreview{Protocol: "wireguard"}
	if len(raw) == 0 {
		return preview, fmt.Errorf("WireGuard config is empty")
	}
	if len(raw) > MaxImportedConfigBytes || strings.IndexByte(string(raw), 0) >= 0 {
		return preview, fmt.Errorf("WireGuard config is invalid or too large")
	}
	section := ""
	interfaces := 0
	peers := 0
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(strings.Trim(line, "[]")))
			if section == "interface" {
				interfaces++
			}
			if section == "peer" {
				peers++
			}
			if section != "interface" && section != "peer" {
				return preview, fmt.Errorf("unsupported WireGuard section %q", section)
			}
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || section == "" {
			return preview, fmt.Errorf("invalid WireGuard configuration line")
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		if key == "preup" || key == "postup" || key == "predown" || key == "postdown" || key == "saveconfig" {
			return preview, fmt.Errorf("WireGuard hook %q is not allowed in a managed profile", key)
		}
		if section == "interface" && key == "fwmark" {
			return preview, fmt.Errorf("WireGuard FwMark is managed by the platform and cannot be imported")
		}
		switch section + "." + key {
		case "interface.privatekey":
			preview.HasPrivateKey = value != ""
		case "interface.address":
			for _, item := range strings.Split(value, ",") {
				if _, _, err := net.ParseCIDR(strings.TrimSpace(item)); err != nil {
					return preview, fmt.Errorf("WireGuard interface address is invalid")
				}
				preview.Addresses = append(preview.Addresses, strings.TrimSpace(item))
			}
		case "peer.presharedkey":
			preview.HasPresharedKey = value != ""
		case "peer.endpoint":
			host, portText, err := net.SplitHostPort(value)
			if err != nil {
				return preview, fmt.Errorf("WireGuard endpoint is invalid: %w", err)
			}
			port, err := strconv.Atoi(portText)
			if err != nil || port < 1 || port > 65535 {
				return preview, fmt.Errorf("WireGuard endpoint port is invalid")
			}
			if net.ParseIP(host) == nil && !validDNSName(host) {
				return preview, fmt.Errorf("WireGuard endpoint host is invalid")
			}
			if preview.EndpointHost == "" {
				preview.EndpointHost, preview.EndpointPort = host, port
			}
		case "peer.allowedips":
			for _, item := range strings.Split(value, ",") {
				allowedIP := strings.TrimSpace(item)
				if _, _, err := net.ParseCIDR(allowedIP); err != nil {
					return preview, fmt.Errorf("WireGuard AllowedIPs value is invalid")
				}
				preview.AllowedIPs = append(preview.AllowedIPs, allowedIP)
				if allowedIP == "0.0.0.0/0" {
					preview.HasDefaultRoute = true
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return preview, err
	}
	if interfaces != 1 {
		return preview, fmt.Errorf("WireGuard config must contain exactly one Interface section")
	}
	if peers != 1 {
		return preview, fmt.Errorf("managed WireGuard external egress requires exactly one Peer section")
	}
	if !preview.HasPrivateKey {
		preview.RequiredSecrets = append(preview.RequiredSecrets, "private_key")
	}
	if preview.EndpointHost == "" {
		return preview, fmt.Errorf("WireGuard config does not contain a usable peer endpoint")
	}
	if len(preview.AllowedIPs) == 0 {
		return preview, fmt.Errorf("WireGuard config does not define AllowedIPs")
	}
	return preview, nil
}
