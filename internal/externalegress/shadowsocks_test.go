package externalegress

import "testing"

func TestParseShadowsocksSIP002URL(t *testing.T) {
	preview, err := ParseShadowsocks([]byte("ss://YWVzLTI1Ni1nY206c3VwZXItc2VjcmV0@ss.example.com:8388#provider"))
	if err != nil {
		t.Fatalf("ParseShadowsocks returned error: %v", err)
	}
	if preview.EndpointHost != "ss.example.com" || preview.EndpointPort != 8388 || preview.Method != "aes-256-gcm" || !preview.HasPassword {
		t.Fatalf("unexpected preview: %#v", preview)
	}
}

func TestParseShadowsocksOpaqueSIP002URLWithFragment(t *testing.T) {
	preview, err := ParseShadowsocks([]byte("ss://YWVzLTI1Ni1nY206c3VwZXItc2VjcmV0QHNzLmV4YW1wbGUuY29tOjgzODg=#provider"))
	if err != nil {
		t.Fatalf("ParseShadowsocks returned error: %v", err)
	}
	if preview.EndpointHost != "ss.example.com" || preview.EndpointPort != 8388 || preview.Method != "aes-256-gcm" {
		t.Fatalf("unexpected preview: %#v", preview)
	}
}

func TestParseShadowsocksJSONWithSeparatePassword(t *testing.T) {
	preview, err := ParseShadowsocks([]byte(`{"server":"ss.example.com","server_port":443,"method":"2022-blake3-aes-256-gcm"}`))
	if err != nil {
		t.Fatalf("ParseShadowsocks returned error: %v", err)
	}
	if len(preview.RequiredSecrets) != 1 || preview.RequiredSecrets[0] != "password" {
		t.Fatalf("required secrets = %#v", preview.RequiredSecrets)
	}
}

func TestParseShadowsocksJSONPreservesPasswordWhitespace(t *testing.T) {
	preview, err := ParseShadowsocks([]byte(`{"server":"ss.example.com","server_port":8388,"method":"aes-256-gcm","password":" pass phrase "}`))
	if err != nil {
		t.Fatalf("ParseShadowsocks returned error: %v", err)
	}
	if preview.Password != " pass phrase " {
		t.Fatalf("password = %q", preview.Password)
	}
}

func TestParseShadowsocksRejectsPluginAndLegacyCipher(t *testing.T) {
	for _, raw := range []string{
		`{"server":"ss.example.com","server_port":443,"method":"aes-256-cfb","password":"secret"}`,
		`{"server":"ss.example.com","server_port":443,"method":"aes-256-gcm","password":"secret","plugin":"/tmp/run"}`,
		`{"server":"ss.example.com","server_port":443,"method":"aes-256-gcm","password":"secret","post_up":"run"}`,
		`{"server":"ss.example.com","server_port":443.5,"method":"aes-256-gcm","password":"secret"}`,
		"ss://YWVzLTI1Ni1nY206c3VwZXItc2VjcmV0@ss.example.com:8388?mode=unsafe",
	} {
		if _, err := ParseShadowsocks([]byte(raw)); err == nil {
			t.Fatalf("expected unsafe Shadowsocks profile to be rejected: %s", raw)
		}
	}
}
