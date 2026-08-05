package main

import (
	"context"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
)

// driverStateStore is the persistence boundary used while materializing
// protocol-specific instance and client state.
type driverStateStore interface {
	CreatePlatformServicePKIRoot(ctx context.Context, serviceCode, profile, commonName, caCertRefID, caKeyRefID string) (domain.PlatformServicePKIRoot, error)
	CreateSecretRef(ctx context.Context, secretType string, rawValue []byte, meta map[string]any) (domain.SecretRef, error)
	EnsureXrayServiceAccessUUID(ctx context.Context, accessID string) (map[string]any, error)
	FindNodeInstanceByService(ctx context.Context, nodeID, serviceCode string) (domain.Instance, error)
	GetActivePlatformServicePKIRoot(ctx context.Context, serviceCode, profile string) (domain.PlatformServicePKIRoot, error)
	GetInstanceWithSpec(ctx context.Context, instanceID string) (domain.Instance, error)
	ListProvisioningAccessesByInstance(ctx context.Context, instanceID string) ([]domain.ProvisioningAccess, error)
	ReplaceInstanceSpec(ctx context.Context, instanceID, source string, spec map[string]any) (domain.InstanceRevision, error)
	ResolveSecretValue(ctx context.Context, secretRefID string) (domain.SecretRef, []byte, error)
	ResolveXrayVLESSEgress(ctx context.Context, instanceID, requestedEgressNodeID string) (postgres.XrayVLESSEgressResolution, error)
	UpdateServiceAccessMetadata(ctx context.Context, accessID string, metadata map[string]any) error
}
