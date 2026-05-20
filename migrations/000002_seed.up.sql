insert into service_definitions(id,code,name,category,tier,supports_accounts,supports_artifacts,enabled)
values
  (gen_random_uuid(),'openvpn','OpenVPN','vpn','A',true,true,true),
  (gen_random_uuid(),'xray','Xray','vpn','A',true,true,true),
  (gen_random_uuid(),'nginx','Nginx','edge','A',false,false,true),
  (gen_random_uuid(),'ipsec','IPsec / L2TP / IKE','vpn','A',true,true,true),
  (gen_random_uuid(),'wireguard','WireGuard','vpn','B',true,true,true),
  (gen_random_uuid(),'shadowsocks','Shadowsocks','proxy','B',true,true,true),
  (gen_random_uuid(),'mtproto','MTProto','proxy','C',true,true,true),
  (gen_random_uuid(),'http_proxy','HTTP Proxy','proxy','C',true,false,true)
on conflict(code) do nothing;

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.seed','platform','initial service catalog seeded','{}'::jsonb,now())
on conflict do nothing;
