insert into service_pack_templates(
  key,
  label,
  description,
  base_name_template,
  endpoint_hint,
  requires_endpoint_host,
  platform_notes_json,
  recommendations_json,
  components_json,
  status,
  source,
  version,
  display_order,
  created_at,
  updated_at
) values (
  'default_access_suite',
  'Default Remote Access Suite',
  'Creates a baseline multi-protocol remote-access suite: VLESS + OpenVPN TCP pair, standalone VLESS, OpenVPN UDP, Shadowsocks and WireGuard.',
  'edge-access',
  'access.example.com',
  true,
  $json$[
    "Template does not store runtime secrets: Reality keys, WireGuard private key and Shadowsocks password are generated during revision/apply and stored as secret refs.",
    "Components use distinct listen ports so the pack can be created on one node and one endpoint host."
  ]$json$::jsonb,
  $json$[
    "Verify DNS, firewall/NAT and conflict-free ports on the selected node before production rollout.",
    "Keep WireGuard/OpenVPN address pools unique across service instances.",
    "Use a valid endpoint host/SNI for the public VLESS listener on 443; the second VLESS listener uses 8443 to avoid a port conflict."
  ]$json$::jsonb,
  $json$[
    {
      "label": "VLESS TCP Edge",
      "description": "VLESS/Reality component for the VLESS + OpenVPN TCP pair.",
      "service_code": "xray-core",
      "preset_key": "reality_tcp",
      "name_suffix": "vless-tcp-edge",
      "slug_suffix": "vless-tcp-edge",
      "endpoint_port": 443,
      "requires_endpoint_host": true,
      "spec": {
        "service_profile": "reality_tcp",
        "security": "reality",
        "network": "tcp",
        "dest": "www.cloudflare.com:443",
        "fingerprint": "chrome",
        "auto_generate_reality_keys": true,
        "config_mode": "0640"
      }
    },
    {
      "label": "OpenVPN TCP Companion",
      "description": "OpenVPN TCP component for the VLESS + OpenVPN TCP pair.",
      "service_code": "openvpn",
      "preset_key": "tcp_11994",
      "name_suffix": "openvpn-tcp",
      "slug_suffix": "openvpn-tcp",
      "endpoint_port": 11994,
      "requires_endpoint_host": true,
      "spec": {
        "service_profile": "tcp_11994",
        "pki_scope": "platform",
        "pki_profile": "default",
        "proto": "tcp",
        "dev": "tun",
        "server_network": "10.8.0.0",
        "server_netmask": "255.255.255.0",
        "config_mode": "0644"
      }
    },
    {
      "label": "VLESS Standalone",
      "description": "Standalone VLESS/Reality instance on an alternative TCP port.",
      "service_code": "xray-core",
      "preset_key": "reality_tcp",
      "name_suffix": "vless",
      "slug_suffix": "vless",
      "endpoint_port": 8443,
      "requires_endpoint_host": true,
      "spec": {
        "service_profile": "reality_tcp",
        "security": "reality",
        "network": "tcp",
        "dest": "www.cloudflare.com:443",
        "fingerprint": "chrome",
        "auto_generate_reality_keys": true,
        "config_mode": "0640"
      }
    },
    {
      "label": "OpenVPN UDP",
      "description": "Classic OpenVPN UDP baseline.",
      "service_code": "openvpn",
      "preset_key": "udp_1194",
      "name_suffix": "openvpn-udp",
      "slug_suffix": "openvpn-udp",
      "endpoint_port": 1194,
      "requires_endpoint_host": true,
      "spec": {
        "service_profile": "udp_1194",
        "pki_scope": "platform",
        "pki_profile": "default",
        "proto": "udp",
        "dev": "tun",
        "server_network": "10.9.0.0",
        "server_netmask": "255.255.255.0",
        "config_mode": "0644"
      }
    },
    {
      "label": "Shadowsocks",
      "description": "Standalone Shadowsocks chacha20-ietf-poly1305 baseline.",
      "service_code": "shadowsocks",
      "preset_key": "chacha_full",
      "name_suffix": "shadowsocks",
      "slug_suffix": "shadowsocks",
      "endpoint_port": 8388,
      "requires_endpoint_host": true,
      "spec": {
        "service_profile": "chacha_full",
        "method": "chacha20-ietf-poly1305",
        "mode": "tcp_and_udp",
        "timeout": 300,
        "auto_generate_server_password": true,
        "config_mode": "0640"
      }
    },
    {
      "label": "WireGuard",
      "description": "Standalone WireGuard road-warrior baseline.",
      "service_code": "wireguard",
      "preset_key": "roadwarrior",
      "name_suffix": "wireguard",
      "slug_suffix": "wireguard",
      "endpoint_port": 51820,
      "requires_endpoint_host": true,
      "spec": {
        "service_profile": "roadwarrior",
        "network_cidr": "10.66.0.0/24",
        "server_address": "10.66.0.1/24",
        "client_allowed_ips": "0.0.0.0/0, ::/0",
        "client_dns": "1.1.1.1, 1.0.0.1",
        "persistent_keepalive": 25,
        "auto_generate_server_key": true,
        "config_mode": "0600"
      }
    }
  ]$json$::jsonb,
  'active',
  'default',
  1,
  5,
  now(),
  now()
)
on conflict(key) do nothing;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
values (gen_random_uuid(), 'system', 'migration.default_access_suite_pack', 'service_pack', 'default access suite service pack seeded', '{}'::jsonb, now());
