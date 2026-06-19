package domain

type ProvisioningAccess struct {
	Access   ServiceAccess `json:"access"`
	Client   Client        `json:"client"`
	Instance Instance      `json:"instance"`
}
