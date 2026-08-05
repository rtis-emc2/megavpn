package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
)

type driverStateSpecWrite struct {
	instanceID string
	source     string
	spec       map[string]any
}

type fakeDriverStateStore struct {
	instances        map[string]domain.Instance
	accesses         map[string][]domain.ProvisioningAccess
	serviceInstance  map[string]string
	secretRefs       map[string]domain.SecretRef
	secretValues     map[string][]byte
	pkiRoots         map[string]domain.PlatformServicePKIRoot
	egress           postgres.XrayVLESSEgressResolution
	specWrites       []driverStateSpecWrite
	metadataWrites   int
	secretWrites     int
	failCreateSecret error
}

func newFakeDriverStateStore(instances ...domain.Instance) *fakeDriverStateStore {
	store := &fakeDriverStateStore{
		instances:       map[string]domain.Instance{},
		accesses:        map[string][]domain.ProvisioningAccess{},
		serviceInstance: map[string]string{},
		secretRefs:      map[string]domain.SecretRef{},
		secretValues:    map[string][]byte{},
		pkiRoots:        map[string]domain.PlatformServicePKIRoot{},
	}
	for _, instance := range instances {
		instance.Spec = cloneMapLocal(instance.Spec)
		store.instances[instance.ID] = instance
		store.serviceInstance[instance.NodeID+"\x00"+instance.ServiceCode] = instance.ID
	}
	return store
}

func (s *fakeDriverStateStore) CreatePlatformServicePKIRoot(_ context.Context, serviceCode, profile, commonName, caCertRefID, caKeyRefID string) (domain.PlatformServicePKIRoot, error) {
	root := domain.PlatformServicePKIRoot{
		ID:                "pki-" + profile,
		ServiceCode:       serviceCode,
		PKIProfile:        profile,
		Status:            "active",
		CACertSecretRefID: caCertRefID,
		CAKeySecretRefID:  caKeyRefID,
		CommonName:        commonName,
	}
	s.pkiRoots[serviceCode+"\x00"+profile] = root
	return root, nil
}

func (s *fakeDriverStateStore) CreateSecretRef(_ context.Context, secretType string, rawValue []byte, meta map[string]any) (domain.SecretRef, error) {
	if s.failCreateSecret != nil {
		return domain.SecretRef{}, s.failCreateSecret
	}
	s.secretWrites++
	id := fmt.Sprintf("secret-%d", s.secretWrites)
	ref := domain.SecretRef{ID: id, SecretType: secretType, Meta: cloneMapLocal(meta)}
	s.secretRefs[id] = ref
	s.secretValues[id] = append([]byte(nil), rawValue...)
	return ref, nil
}

func (s *fakeDriverStateStore) EnsureXrayServiceAccessUUID(_ context.Context, accessID string) (map[string]any, error) {
	metadata := s.accessMetadata(accessID)
	if firstNonEmpty(stringify(metadata["xray_uuid"])) == "" {
		metadata["xray_uuid"] = "00000000-0000-4000-8000-" + fmt.Sprintf("%012d", len(accessID))
	}
	s.setAccessMetadata(accessID, metadata)
	return cloneMapLocal(metadata), nil
}

func (s *fakeDriverStateStore) FindNodeInstanceByService(_ context.Context, nodeID, serviceCode string) (domain.Instance, error) {
	id := s.serviceInstance[nodeID+"\x00"+serviceCode]
	if id == "" {
		return domain.Instance{}, pgx.ErrNoRows
	}
	return s.GetInstanceWithSpec(context.Background(), id)
}

func (s *fakeDriverStateStore) GetActivePlatformServicePKIRoot(_ context.Context, serviceCode, profile string) (domain.PlatformServicePKIRoot, error) {
	root, ok := s.pkiRoots[serviceCode+"\x00"+profile]
	if !ok {
		return domain.PlatformServicePKIRoot{}, pgx.ErrNoRows
	}
	return root, nil
}

func (s *fakeDriverStateStore) GetInstanceWithSpec(_ context.Context, instanceID string) (domain.Instance, error) {
	instance, ok := s.instances[instanceID]
	if !ok {
		return domain.Instance{}, pgx.ErrNoRows
	}
	instance.Spec = cloneMapLocal(instance.Spec)
	return instance, nil
}

func (s *fakeDriverStateStore) ListProvisioningAccessesByInstance(_ context.Context, instanceID string) ([]domain.ProvisioningAccess, error) {
	items := s.accesses[instanceID]
	out := make([]domain.ProvisioningAccess, len(items))
	for i, item := range items {
		out[i] = item
		out[i].Access.Metadata = cloneMapLocal(item.Access.Metadata)
		out[i].Access.Policy = cloneMapLocal(item.Access.Policy)
		out[i].Instance.Spec = cloneMapLocal(item.Instance.Spec)
	}
	return out, nil
}

func (s *fakeDriverStateStore) ReplaceInstanceSpec(_ context.Context, instanceID, source string, spec map[string]any) (domain.InstanceRevision, error) {
	instance, ok := s.instances[instanceID]
	if !ok {
		return domain.InstanceRevision{}, pgx.ErrNoRows
	}
	copySpec := cloneMapLocal(spec)
	instance.Spec = copySpec
	s.instances[instanceID] = instance
	s.specWrites = append(s.specWrites, driverStateSpecWrite{instanceID: instanceID, source: source, spec: cloneMapLocal(copySpec)})
	return domain.InstanceRevision{ID: fmt.Sprintf("revision-%d", len(s.specWrites)), InstanceID: instanceID, Source: source, Spec: cloneMapLocal(copySpec)}, nil
}

func (s *fakeDriverStateStore) ResolveSecretValue(_ context.Context, secretRefID string) (domain.SecretRef, []byte, error) {
	ref, ok := s.secretRefs[secretRefID]
	if !ok {
		return domain.SecretRef{}, nil, pgx.ErrNoRows
	}
	return ref, append([]byte(nil), s.secretValues[secretRefID]...), nil
}

func (s *fakeDriverStateStore) ResolveXrayVLESSEgress(_ context.Context, instanceID, _ string) (postgres.XrayVLESSEgressResolution, error) {
	if s.egress.Mode != "" {
		return s.egress, nil
	}
	instance := s.instances[instanceID]
	return postgres.XrayVLESSEgressResolution{Mode: "local_breakout", CurrentNodeID: instance.NodeID}, nil
}

func (s *fakeDriverStateStore) UpdateServiceAccessMetadata(_ context.Context, accessID string, metadata map[string]any) error {
	s.metadataWrites++
	s.setAccessMetadata(accessID, metadata)
	return nil
}

func (s *fakeDriverStateStore) accessMetadata(accessID string) map[string]any {
	for _, items := range s.accesses {
		for _, item := range items {
			if item.Access.ID == accessID {
				return cloneMapLocal(item.Access.Metadata)
			}
		}
	}
	return map[string]any{}
}

func (s *fakeDriverStateStore) setAccessMetadata(accessID string, metadata map[string]any) {
	for instanceID, items := range s.accesses {
		for i := range items {
			if items[i].Access.ID == accessID {
				items[i].Access.Metadata = cloneMapLocal(metadata)
			}
		}
		s.accesses[instanceID] = items
	}
}

func testInstance(id, serviceCode string, spec map[string]any) domain.Instance {
	return domain.Instance{
		ID:           id,
		NodeID:       "node-1",
		ServiceCode:  serviceCode,
		Name:         id,
		Slug:         id,
		EndpointHost: "vpn.example.com",
		EndpointPort: 443,
		Spec:         spec,
	}
}

func testAccess(instance domain.Instance, accessID string) domain.ProvisioningAccess {
	return domain.ProvisioningAccess{
		Access:   domain.ServiceAccess{ID: accessID, InstanceID: instance.ID, Status: "active", Policy: map[string]any{}, Metadata: map[string]any{}},
		Client:   domain.Client{ID: "client-" + accessID, Username: "user-" + accessID, Status: "active"},
		Instance: instance,
	}
}

func TestEnsureXrayInstanceDriverStateStoresRealityPrivateKeyByReference(t *testing.T) {
	instance := testInstance("xray-1", "xray-core", map[string]any{"security": "reality"})
	store := newFakeDriverStateStore(instance)
	store.accesses[instance.ID] = []domain.ProvisioningAccess{testAccess(instance, "access-1")}

	if err := ensureXrayInstanceDriverState(context.Background(), store, instance.ID); err != nil {
		t.Fatalf("ensure xray state: %v", err)
	}
	spec := store.instances[instance.ID].Spec
	privateRefID := stringify(spec["reality_private_key_secret_ref_id"])
	if privateRefID == "" || len(store.secretValues[privateRefID]) == 0 {
		t.Fatal("Reality private key was not persisted through a secret reference")
	}
	if _, exists := spec["reality_private_key"]; exists {
		t.Fatal("Reality private key remained in instance spec")
	}
	if got := stringify(spec["reality_public_key"]); got == "" {
		t.Fatal("Reality public key was not materialized")
	}
	if got := stringify(spec["short_id"]); len(got) != 8 {
		t.Fatalf("short_id length = %d, want 8", len(got))
	}
	clients, _ := spec["managed_clients"].([]any)
	if len(clients) != 1 || stringify(clients[0].(map[string]any)["id"]) == "" {
		t.Fatalf("managed_clients = %#v, want one UUID-bearing client", clients)
	}

	secretWrites := store.secretWrites
	if err := ensureXrayInstanceDriverState(context.Background(), store, instance.ID); err != nil {
		t.Fatalf("ensure xray state twice: %v", err)
	}
	if store.secretWrites != secretWrites {
		t.Fatalf("second ensure created %d additional secrets", store.secretWrites-secretWrites)
	}
}

func TestEnsureXrayInstanceDriverStateMaterializesGroupEgress(t *testing.T) {
	instance := testInstance("xray-1", "xray-core", map[string]any{
		"security": "none",
		"vless_groups": []any{map[string]any{
			"key":            "out_usa",
			"label":          "USA exit",
			"egress_mode":    "egress_node",
			"egress_node_id": "node-2",
		}},
	})
	store := newFakeDriverStateStore(instance)
	store.egress = postgres.XrayVLESSEgressResolution{
		Mode:          "managed_backhaul",
		CurrentNodeID: instance.NodeID,
		EgressNodeID:  "node-2",
		SendThrough:   "10.240.0.1",
		InterfaceName: "mgbh-test",
		RoutingTable:  "43439",
	}
	access := testAccess(instance, "access-1")
	access.Access.Metadata["xray_uuid"] = "11111111-1111-4111-8111-111111111111"
	access.Access.Policy["vless_group"] = "out_usa"
	store.accesses[instance.ID] = []domain.ProvisioningAccess{access}

	if err := ensureXrayInstanceDriverState(context.Background(), store, instance.ID); err != nil {
		t.Fatalf("ensure Xray group egress: %v", err)
	}
	spec := store.instances[instance.ID].Spec
	groups := spec["vless_groups"].([]any)
	group := groups[0].(map[string]any)
	if got := stringify(group["outbound_tag"]); got != "egress-out_usa" {
		t.Fatalf("outbound_tag = %q, want egress-out_usa", got)
	}
	outbound := group["outbound"].(map[string]any)
	if got := stringify(outbound["sendThrough"]); got != "10.240.0.1" {
		t.Fatalf("group sendThrough = %q, want 10.240.0.1", got)
	}
	clients := spec["managed_clients"].([]any)
	if got := stringify(clients[0].(map[string]any)["vless_group"]); got != "out_usa" {
		t.Fatalf("client vless_group = %q, want out_usa", got)
	}
}

func TestEnsureOpenVPNStateIsIdempotentAndUsesSecretReferences(t *testing.T) {
	instance := testInstance("openvpn-1", "openvpn", map[string]any{"pki_scope": "platform", "pki_profile": "default"})
	instance.EndpointPort = 1194
	store := newFakeDriverStateStore(instance)
	record := testAccess(instance, "access-1")
	store.accesses[instance.ID] = []domain.ProvisioningAccess{record}

	if err := ensureOpenVPNInstanceAndClientState(context.Background(), store, &record); err != nil {
		t.Fatalf("ensure OpenVPN state: %v", err)
	}
	spec := store.instances[instance.ID].Spec
	for _, key := range []string{"ca_cert_secret_ref_id", "ca_key_secret_ref_id", "server_cert_secret_ref_id", "server_key_secret_ref_id"} {
		if stringify(spec[key]) == "" {
			t.Fatalf("%s was not materialized", key)
		}
	}
	for _, key := range []string{"ca_pem", "ca_key_pem", "server_key", "server_key_pem"} {
		if _, exists := spec[key]; exists {
			t.Fatalf("plaintext key material %s remained in OpenVPN spec", key)
		}
	}
	metadata := store.accessMetadata(record.Access.ID)
	for _, key := range []string{"openvpn_client_cert_secret_ref_id", "openvpn_client_key_secret_ref_id", "openvpn_ca_cert_secret_ref_id"} {
		if stringify(metadata[key]) == "" {
			t.Fatalf("client metadata %s was not materialized", key)
		}
	}

	secretWrites := store.secretWrites
	if err := ensureOpenVPNInstanceAndClientState(context.Background(), store, &record); err != nil {
		t.Fatalf("ensure OpenVPN state twice: %v", err)
	}
	if store.secretWrites != secretWrites {
		t.Fatalf("second ensure created %d additional secrets", store.secretWrites-secretWrites)
	}
}

func TestEnsureWireGuardStateKeepsPrivateKeysOutOfSpecsAndMetadata(t *testing.T) {
	instance := testInstance("wireguard-1", "wireguard", map[string]any{"network_cidr": "10.66.0.0/24"})
	instance.EndpointPort = 51820
	store := newFakeDriverStateStore(instance)
	store.accesses[instance.ID] = []domain.ProvisioningAccess{testAccess(instance, "access-1")}

	if err := ensureWireGuardInstanceDriverState(context.Background(), store, instance.ID); err != nil {
		t.Fatalf("ensure WireGuard state: %v", err)
	}
	spec := store.instances[instance.ID].Spec
	if stringify(spec["server_private_key_secret_ref_id"]) == "" {
		t.Fatal("server private key secret reference is missing")
	}
	if _, exists := spec["server_private_key"]; exists {
		t.Fatal("server private key remained in instance spec")
	}
	metadata := store.accessMetadata("access-1")
	if stringify(metadata["wireguard_client_private_key_secret_ref_id"]) == "" {
		t.Fatal("client private key secret reference is missing")
	}
	if _, exists := metadata["wireguard_client_private_key"]; exists {
		t.Fatal("client private key remained in service access metadata")
	}
	peers, _ := spec["managed_peers"].([]any)
	if len(peers) != 1 || stringify(peers[0].(map[string]any)["public_key"]) == "" {
		t.Fatalf("managed_peers = %#v, want one public-key peer", peers)
	}

	firstMetadata := cloneMapLocal(metadata)
	secretWrites := store.secretWrites
	if err := ensureWireGuardInstanceDriverState(context.Background(), store, instance.ID); err != nil {
		t.Fatalf("ensure WireGuard state twice: %v", err)
	}
	if store.secretWrites != secretWrites {
		t.Fatalf("second ensure created %d additional secrets", store.secretWrites-secretWrites)
	}
	if got := store.accessMetadata("access-1"); !reflect.DeepEqual(got, firstMetadata) {
		t.Fatalf("second ensure changed client credentials:\nfirst=%#v\nsecond=%#v", firstMetadata, got)
	}
}

func TestEnsureWireGuardStateRotatesClientCredentialsOnlyWhenRequested(t *testing.T) {
	instance := testInstance("wireguard-1", "wireguard", map[string]any{"network_cidr": "10.66.0.0/24"})
	store := newFakeDriverStateStore(instance)
	store.accesses[instance.ID] = []domain.ProvisioningAccess{testAccess(instance, "access-1")}
	if err := ensureWireGuardInstanceDriverState(context.Background(), store, instance.ID); err != nil {
		t.Fatalf("initial ensure: %v", err)
	}
	before := store.accessMetadata("access-1")
	beforeRef := stringify(before["wireguard_client_private_key_secret_ref_id"])
	beforePublic := stringify(before["wireguard_client_public_key"])

	before["rotate_credentials"] = true
	store.setAccessMetadata("access-1", before)
	if err := ensureWireGuardInstanceDriverState(context.Background(), store, instance.ID); err != nil {
		t.Fatalf("rotating ensure: %v", err)
	}
	after := store.accessMetadata("access-1")
	if got := stringify(after["wireguard_client_private_key_secret_ref_id"]); got == "" || got == beforeRef {
		t.Fatalf("client private key ref was not rotated: before=%q after=%q", beforeRef, got)
	}
	if got := stringify(after["wireguard_client_public_key"]); got == "" || got == beforePublic {
		t.Fatalf("client public key was not rotated: before=%q after=%q", beforePublic, got)
	}
	if _, exists := after["rotate_credentials"]; exists {
		t.Fatal("rotate_credentials marker was not cleared")
	}
}

func TestEnsureProxyDriverStatesPreserveCredentialsUntilRotation(t *testing.T) {
	tests := []struct {
		name       string
		service    string
		ensure     func(context.Context, driverStateStore, string) error
		credential string
		runtimeKey string
	}{
		{name: "shadowsocks", service: "shadowsocks", ensure: ensureShadowsocksInstanceDriverState, credential: "shadowsocks_password", runtimeKey: "managed_accounts"},
		{name: "http proxy", service: "http-proxy", ensure: ensureHTTPProxyInstanceDriverState, credential: "http_proxy_password", runtimeKey: "managed_accounts"},
		{name: "MTProto", service: "mtproto", ensure: ensureMTProtoInstanceDriverState, credential: "mtproto_secret", runtimeKey: "managed_users"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := testInstance(tt.name+"-1", tt.service, map[string]any{})
			store := newFakeDriverStateStore(instance)
			store.accesses[instance.ID] = []domain.ProvisioningAccess{testAccess(instance, "access-1")}

			if err := tt.ensure(context.Background(), store, instance.ID); err != nil {
				t.Fatalf("first ensure: %v", err)
			}
			firstCredential := stringify(store.accessMetadata("access-1")[tt.credential])
			if firstCredential == "" {
				t.Fatalf("%s was not generated", tt.credential)
			}
			if runtime, ok := store.instances[instance.ID].Spec[tt.runtimeKey].([]any); !ok || len(runtime) != 1 {
				t.Fatalf("%s = %#v, want one runtime account", tt.runtimeKey, store.instances[instance.ID].Spec[tt.runtimeKey])
			}

			if err := tt.ensure(context.Background(), store, instance.ID); err != nil {
				t.Fatalf("second ensure: %v", err)
			}
			if secondCredential := stringify(store.accessMetadata("access-1")[tt.credential]); secondCredential != firstCredential {
				t.Fatalf("credential changed without rotation: first=%q second=%q", firstCredential, secondCredential)
			}
		})
	}
}

func TestEnsureIPSecL2TPStateStoresPSKByReference(t *testing.T) {
	ipsec := testInstance("ipsec-1", "ipsec", map[string]any{})
	ipsec.EndpointPort = 500
	xl2tpd := testInstance("xl2tpd-1", "xl2tpd", map[string]any{"listen_port": 1701})
	store := newFakeDriverStateStore(ipsec, xl2tpd)
	record := testAccess(ipsec, "access-1")
	store.accesses[ipsec.ID] = []domain.ProvisioningAccess{record}

	targets, err := ensureIPSecL2TPInstanceDriverState(context.Background(), store, &record)
	if err != nil {
		t.Fatalf("ensure IPsec/L2TP state: %v", err)
	}
	if targets[ipsec.ID] != "ipsec" || targets[xl2tpd.ID] != "xl2tpd" {
		t.Fatalf("targets = %#v", targets)
	}
	spec := store.instances[ipsec.ID].Spec
	pskRefID := stringify(spec["psk_secret_ref_id"])
	if pskRefID == "" || len(store.secretValues[pskRefID]) == 0 {
		t.Fatal("IPsec PSK was not persisted through a secret reference")
	}
	if _, exists := spec["psk"]; exists {
		t.Fatal("IPsec PSK remained in instance spec")
	}
	if got := stringify(store.accessMetadata("access-1")["ipsec_psk_secret_ref_id"]); got != pskRefID {
		t.Fatalf("client PSK ref = %q, want %q", got, pskRefID)
	}
	if entries := stringify(store.instances[xl2tpd.ID].Spec["chap_secrets_entries"]); !strings.Contains(entries, "user-access-1") {
		t.Fatalf("xl2tpd CHAP material was not generated: %q", entries)
	}

	secretWrites := store.secretWrites
	if _, err := ensureIPSecL2TPInstanceDriverState(context.Background(), store, &record); err != nil {
		t.Fatalf("ensure IPsec/L2TP state twice: %v", err)
	}
	if store.secretWrites != secretWrites {
		t.Fatalf("second ensure created %d additional secrets", store.secretWrites-secretWrites)
	}
}

func TestDriverStateSecretFailureDoesNotPublishRevision(t *testing.T) {
	instance := testInstance("wireguard-1", "wireguard", map[string]any{"network_cidr": "10.66.0.0/24"})
	store := newFakeDriverStateStore(instance)
	store.failCreateSecret = errors.New("secret store unavailable")

	err := ensureWireGuardInstanceDriverState(context.Background(), store, instance.ID)
	if err == nil || !strings.Contains(err.Error(), "secret store unavailable") {
		t.Fatalf("error = %v, want secret store failure", err)
	}
	if len(store.specWrites) != 0 {
		t.Fatalf("published %d revisions after secret persistence failure", len(store.specWrites))
	}
	if got := store.instances[instance.ID].Spec; !reflect.DeepEqual(got, map[string]any{"network_cidr": "10.66.0.0/24"}) {
		t.Fatalf("stored spec changed after failed ensure: %#v", got)
	}
}
