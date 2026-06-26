package domain

import "time"

type AddressPoolSpace struct {
	ID               string    `json:"id"`
	Key              string    `json:"key"`
	Label            string    `json:"label"`
	Description      string    `json:"description"`
	Family           string    `json:"family"`
	BaseCIDR         string    `json:"base_cidr"`
	StartCIDR        string    `json:"start_cidr"`
	AllocationPrefix int       `json:"allocation_prefix"`
	ServiceScope     string    `json:"service_scope"`
	RoutingEnabled   bool      `json:"routing_enabled"`
	Status           string    `json:"status"`
	DisplayOrder     int       `json:"display_order"`
	Capacity         int       `json:"capacity"`
	Used             int       `json:"used"`
	Free             int       `json:"free"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type AddressPoolAllocation struct {
	ID             string         `json:"id"`
	PoolSpaceID    string         `json:"pool_space_id"`
	PoolSpaceKey   string         `json:"pool_space_key"`
	PoolSpaceLabel string         `json:"pool_space_label"`
	CIDR           string         `json:"cidr"`
	NodeID         *string        `json:"node_id,omitempty"`
	NodeName       string         `json:"node_name"`
	InstanceID     *string        `json:"instance_id,omitempty"`
	InstanceName   string         `json:"instance_name"`
	ServiceCode    string         `json:"service_code"`
	Purpose        string         `json:"purpose"`
	Status         string         `json:"status"`
	RouteExport    bool           `json:"route_export"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type AddressPoolInventory struct {
	Spaces      []AddressPoolSpace      `json:"spaces"`
	Allocations []AddressPoolAllocation `json:"allocations"`
}
