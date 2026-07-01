package domain

import "time"

type FirewallAddressList struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	Scope       string    `json:"scope"`
	Status      string    `json:"status"`
	EntryCount  int       `json:"entry_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type FirewallAddressEntry struct {
	ID        string    `json:"id"`
	ListID    string    `json:"list_id"`
	ListKey   string    `json:"list_key"`
	Value     string    `json:"value"`
	ValueType string    `json:"value_type"`
	Label     string    `json:"label"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type FirewallPolicy struct {
	ID                   string    `json:"id"`
	Key                  string    `json:"key"`
	Label                string    `json:"label"`
	Description          string    `json:"description"`
	Scope                string    `json:"scope"`
	NodeID               *string   `json:"node_id,omitempty"`
	NodeName             string    `json:"node_name"`
	DefaultInputPolicy   string    `json:"default_input_policy"`
	DefaultForwardPolicy string    `json:"default_forward_policy"`
	DefaultOutputPolicy  string    `json:"default_output_policy"`
	Status               string    `json:"status"`
	RuleCount            int       `json:"rule_count"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type FirewallRule struct {
	ID         string         `json:"id"`
	PolicyID   string         `json:"policy_id"`
	Priority   int            `json:"priority"`
	Chain      string         `json:"chain"`
	Action     string         `json:"action"`
	Direction  string         `json:"direction"`
	Protocol   string         `json:"protocol"`
	SrcListID  *string        `json:"src_list_id,omitempty"`
	SrcListKey string         `json:"src_list_key"`
	DstListID  *string        `json:"dst_list_id,omitempty"`
	DstListKey string         `json:"dst_list_key"`
	SrcCIDR    string         `json:"src_cidr"`
	DstCIDR    string         `json:"dst_cidr"`
	SrcPorts   string         `json:"src_ports"`
	DstPorts   string         `json:"dst_ports"`
	StateMatch []string       `json:"state_match"`
	Comment    string         `json:"comment"`
	Enabled    bool           `json:"enabled"`
	Log        bool           `json:"log"`
	Status     string         `json:"status"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

type FirewallNodeState struct {
	ID                string         `json:"id"`
	NodeID            string         `json:"node_id"`
	NodeName          string         `json:"node_name"`
	PolicyID          *string        `json:"policy_id,omitempty"`
	PolicyKey         string         `json:"policy_key"`
	RevisionID        *string        `json:"revision_id,omitempty"`
	DesiredRevisionID *string        `json:"desired_revision_id,omitempty"`
	Status            string         `json:"status"`
	Observed          map[string]any `json:"observed"`
	LastJobID         *string        `json:"last_job_id,omitempty"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type FirewallInventory struct {
	AddressLists []FirewallAddressList  `json:"address_lists"`
	Entries      []FirewallAddressEntry `json:"entries"`
	Policies     []FirewallPolicy       `json:"policies"`
	Rules        []FirewallRule         `json:"rules"`
	NodeStates   []FirewallNodeState    `json:"node_states"`
}
