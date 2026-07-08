export type APIRecord = Record<string, unknown>;

export type AuthUser = {
  id?: string;
  username?: string;
  email?: string;
  display_name?: string;
  status?: string;
};

export type AuthSession = {
  id?: string;
  expires_at?: string;
};

export type AuthPayload = {
  user: AuthUser | null;
  session: AuthSession | null;
  roles: string[];
  permissions: string[];
};

export type ReadyStatus = {
  status?: string;
  service?: string;
  version?: string;
  production_mode?: boolean;
  preflight_status?: string;
  time?: string;
};

export type VersionInfo = {
  service?: string;
  version?: string;
  public_base_url?: string;
};

export type Dashboard = APIRecord & {
  version?: string;
  nodes_total?: number;
  instances_total?: number;
  clients_total?: number;
  jobs_active?: number;
  jobs_failed?: number;
};

export type NodeEntity = APIRecord & {
  id: string;
  name?: string;
  role?: string;
  address?: string;
  status?: string;
  agent_status?: string;
  agent_channel_status?: string;
  last_heartbeat_at?: string;
  latitude?: number;
  longitude?: number;
};

export type ServiceInstance = APIRecord & {
  id: string;
  name?: string;
  slug?: string;
  service_code?: string;
  node_id?: string;
  node_name?: string;
  endpoint_host?: string;
  endpoint_port?: number;
  status?: string;
  current_revision_id?: string;
  last_applied_revision_id?: string;
};

export type ClientAccount = APIRecord & {
  id: string;
  username?: string;
  display_name?: string;
  email?: string;
  status?: string;
  created_at?: string;
};

export type ClientAccessService = APIRecord & {
  service_code: string;
  display_name?: string;
  category?: string;
  status?: string;
  supports_groups?: boolean;
  supports_membership?: boolean;
  supports_materialization?: boolean;
};

export type ClientAccessGroup = APIRecord & {
  id: string;
  service_code?: string;
  group_key?: string;
  display_name?: string;
  status?: string;
  member_count?: number;
  affected_instances?: number;
  scope_mode?: string;
  auto_apply_new_instances?: boolean;
};

export type FirewallPolicy = APIRecord & {
  id: string;
  key?: string;
  label?: string;
  status?: string;
  default_action?: string;
  scope?: string;
};

export type FirewallRule = APIRecord & {
  id: string;
  policy_id?: string;
  priority?: number;
  action?: string;
  protocol?: string;
  chain?: string;
  comment?: string;
};

export type FirewallAddressGroup = APIRecord & {
  id: string;
  key?: string;
  label?: string;
  status?: string;
  entry_count?: number;
};

export type FirewallInventory = {
  address_lists: FirewallAddressGroup[];
  entries: APIRecord[];
  policies: FirewallPolicy[];
  rules: FirewallRule[];
  node_states: APIRecord[];
};

export type TrafficSummary = APIRecord & {
  summary?: APIRecord;
  samples?: APIRecord[];
  collectors?: APIRecord[];
  clients?: APIRecord[];
};

export type Job = APIRecord & {
  id: string;
  type?: string;
  status?: string;
  scope_type?: string;
  scope_id?: string;
  created_at?: string;
  updated_at?: string;
  result?: APIRecord;
};

export type Artifact = APIRecord & {
  id: string;
  client_id?: string;
  artifact_type?: string;
  type?: string;
  status?: string;
  storage_path?: string;
  path?: string;
  size_bytes?: number;
};

export type ShareLink = APIRecord & {
  id: string;
  client_id?: string;
  status?: string;
  token_hint?: string;
  url?: string;
  expires_at?: string;
};

export type BackhaulLink = APIRecord & {
  id: string;
  name?: string;
  status?: string;
  ingress_node_id?: string;
  egress_node_id?: string;
  driver?: string;
  route_enabled?: boolean;
};

export type Certificate = APIRecord & {
  id: string;
  common_name?: string;
  issuer_name?: string;
  status?: string;
  kind?: string;
  source?: string;
  not_after?: string;
};

export type AddressPools = {
  spaces: APIRecord[];
  allocations: APIRecord[];
};
