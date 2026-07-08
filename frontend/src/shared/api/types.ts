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
  enabled?: boolean;
  systemd_unit?: string;
  current_revision_id?: string;
  last_applied_revision_id?: string;
  created_at?: string;
  updated_at?: string;
  spec?: APIRecord;
};

export type ServiceInstanceDetail = ServiceInstance;

export type RuntimeCheck = APIRecord & {
  code?: string;
  display_name?: string;
  signal?: string;
  source?: string;
  required?: boolean;
  status?: string;
  message?: string;
  expected?: unknown;
  observed?: unknown;
};

export type ServiceInstanceRuntimeState = APIRecord & {
  id?: string;
  instance_id?: string;
  node_id?: string | null;
  service_code?: string;
  systemd_unit?: string;
  desired_status?: string;
  runtime_status?: string;
  health_status?: string;
  drift_status?: string;
  health_checks?: RuntimeCheck[];
  health_reasons?: string[];
  drift_reasons?: string[];
  active_state?: string;
  enabled_state?: string;
  config_hash?: string;
  last_job_id?: string | null;
  last_job_type?: string;
  last_job_status?: string;
  applied_revision_id?: string | null;
  observed_revision_id?: string | null;
  endpoint_host?: string;
  endpoint_port?: number;
  listening_ports?: APIRecord[];
  result?: APIRecord;
  error_text?: string;
  agent_reported_at?: string | null;
  checked_at?: string;
  updated_at?: string;
};

export type ServiceInstanceRuntimeObservation = ServiceInstanceRuntimeState & {
  source?: string;
  observed_at?: string;
  received_at?: string;
};

export type ServiceInstanceRevision = APIRecord & {
  id: string;
  instance_id?: string;
  source?: string;
  status?: string;
  rendered_hash?: string;
  revision_no?: number;
  spec?: APIRecord;
  validation_errors?: unknown[];
  created_at?: string;
  applied_at?: string | null;
  is_current?: boolean;
  is_last_applied?: boolean;
};

export type InstanceAccessGroupMaterialization = APIRecord & {
  instance_id?: string;
  instance_name?: string;
  instance_slug?: string;
  service_code?: string;
  available_keys?: string[];
  groups?: Array<APIRecord & {
    key?: string;
    label?: string;
    description?: string;
    status?: string;
    service_code?: string;
    member_count?: number;
    pending_count?: number;
    active_count?: number;
    disabled_count?: number;
    target_instance_id?: string;
  }>;
};

export type InstanceApplyRequest = APIRecord & {
  reason?: string;
};

export type InstanceApplyResult = Job;

export type InstanceRollbackRequest = {
  revision_id?: string;
};

export type InstanceRollbackResult = APIRecord & {
  revision?: ServiceInstanceRevision;
  can_apply?: boolean;
  message?: string;
  issue_count?: number;
};

export type InstanceLifecycleAction = 'start' | 'stop' | 'restart' | 'enable' | 'disable';

export type InstanceLifecycleResult = Job;

export type InstanceDeleteResult = ServiceInstance | (APIRecord & {
  status?: string;
  instance?: ServiceInstance;
});

export type InstanceForceDeleteInput = {
  confirmation: string;
  reason?: string;
};

export type ApiValidationError = APIRecord & {
  error?: string;
  fields?: Record<string, string>;
};

export type ClientAccount = APIRecord & {
  id: string;
  username?: string;
  display_name?: string;
  email?: string;
  status?: string;
  notes?: string;
  expires_at?: string | null;
  created_at?: string;
  updated_at?: string;
  summary?: ClientSummary;
};

export type ClientStatus = 'active' | 'suspended' | 'revoked' | 'expired' | 'deleted';

export type ClientSummary = {
  service_access_count?: number;
  active_service_access_count?: number;
  pending_service_access_count?: number;
  route_count?: number;
  active_route_count?: number;
  artifact_count?: number;
  ready_artifact_count?: number;
  share_link_count?: number;
  active_share_link_count?: number;
  last_artifact_at?: string;
  next_share_link_expires_at?: string;
};

export type ClientDetail = ClientAccount;

export type ClientCreateInput = {
  username: string;
  display_name?: string;
  email?: string;
  notes?: string;
  expires_at?: string | null;
  status?: ClientStatus;
};

export type ClientStatusUpdateInput = {
  status: Extract<ClientStatus, 'active' | 'suspended'>;
};

export type ClientDeleteResult = APIRecord & {
  client_id?: string;
  deleted?: boolean;
  config_cleanup?: APIRecord;
};

export type ClientServiceAccess = APIRecord & {
  id: string;
  client_account_id?: string;
  instance_id?: string;
  status?: string;
  provision_mode?: string;
  policy?: APIRecord;
  metadata?: APIRecord;
  created_at?: string;
  updated_at?: string;
};

export type ClientAccessService = APIRecord & {
  service_code: string;
  display_name?: string;
  description?: string;
  category?: string;
  implemented?: boolean;
  status?: string;
  supports_groups?: boolean;
  supports_policy?: boolean;
  supports_scope?: boolean;
  supports_membership?: boolean;
  supports_materialization?: boolean;
  runtime_service_codes?: string[];
  policy_capabilities?: APIRecord;
};

export type VLESSAccessGroupPolicy = {
  access_mode?: string;
  egress_mode?: string;
  egress_node_id?: string;
  target_instance_id?: string;
  outbound_tag?: string;
  ad_block?: boolean;
  rules?: APIRecord[];
  extra_rules?: APIRecord[];
};

export type ClientAccessGroup = APIRecord & {
  id: string;
  service_code?: string;
  group_key?: string;
  display_name?: string;
  description?: string;
  status?: string;
  policy_json?: VLESSAccessGroupPolicy | APIRecord;
  member_count?: number;
  active_member_count?: number;
  disabled_member_count?: number;
  affected_instances?: number;
  pending_sync_count?: number;
  failed_sync_count?: number;
  applied_sync_count?: number;
  scope_mode?: string;
  auto_apply_new_instances?: boolean;
};

export type ClientAccessGroupInput = {
  service_code: string;
  group_key: string;
  display_name: string;
  description?: string;
  status?: string;
  policy_json?: VLESSAccessGroupPolicy;
  scope_mode?: string;
  auto_apply_new_instances?: boolean;
};

export type ClientAccessGroupMember = APIRecord & {
  client_id: string;
  username?: string;
  display_name?: string;
  email?: string;
  client_status?: string;
  membership_id?: string;
  membership_status?: string;
  group_id?: string;
  group_key?: string;
  group_name?: string;
  service_access_id?: string;
  access_status?: string;
  xray_uuid?: string;
  updated_at?: string;
};

export type ClientAccessGroupMembersPage = {
  group_id?: string;
  service_code?: string;
  group_key?: string;
  status?: string;
  items: ClientAccessGroupMember[];
  total: number;
  limit: number;
  offset: number;
};

export type AvailableClientsForGroupPage = {
  service_code: string;
  assignment: string;
  items: ClientAccessGroupMember[];
  total: number;
  limit: number;
  offset: number;
};

export type ClientAccessGroupMemberQuery = {
  search?: string;
  status?: string;
  limit?: number;
  offset?: number;
};

export type AvailableClientsForGroupQuery = ClientAccessGroupMemberQuery & {
  group_id?: string;
  service_code?: string;
  assignment?: string;
};

export type ClientAccessGroupMembershipRequest = {
  client_ids?: string[];
  client_refs?: string[];
  mode?: 'add_only' | 'add_or_move';
  queue_apply?: boolean;
  build_artifacts?: boolean;
  dry_run?: boolean;
  all_filtered?: boolean;
  filter_search?: string;
  filter_assignment?: string;
  filter_status?: string;
  filter_group_id?: string;
};

export type ClientAccessGroupMembershipFailure = {
  client_id?: string;
  ref?: string;
  error: string;
};

export type ClientAccessGroupMembershipConflict = {
  client_id?: string;
  existing_group?: string;
  target_group?: string;
  reason: string;
};

export type ClientAccessGroupMembershipResult = {
  group_id?: string;
  group_key?: string;
  service_code?: string;
  dry_run?: boolean;
  all_filtered?: boolean;
  created_memberships: number;
  moved_memberships: number;
  skipped_existing: number;
  failed?: ClientAccessGroupMembershipFailure[];
  conflicts?: ClientAccessGroupMembershipConflict[];
  affected_instances?: number;
  materialized_created?: number;
  materialized_updated?: number;
  materialized_disabled?: number;
  sync_job_id?: string;
  apply_job_ids?: string[];
  apply_job_count: number;
  warnings?: string[];
  clients?: ClientAccessGroupMember[];
};

export type ClientAccessGroupScope = {
  group_id: string;
  scope_mode: string;
  auto_apply_new_instances: boolean;
  include_instance_ids?: string[];
  exclude_instance_ids?: string[];
  affected_instances?: number;
  materialized_created?: number;
  materialized_updated?: number;
  materialized_disabled?: number;
  apply_job_count?: number;
  apply_job_ids?: string[];
  warnings?: string[];
};

export type ClientAccessGroupSyncPreview = {
  group_id: string;
  group_key: string;
  service_code: string;
  desired_hash: string;
  affected_instances: number;
  member_count: number;
  pending_instances: number;
  applied_instances: number;
  failed_instances: number;
  instance_ids?: string[];
  warnings?: string[];
};

export type ClientAccessGroupSyncState = {
  group_id: string;
  instance_id: string;
  desired_hash: string;
  last_applied_hash?: string;
  status: string;
  last_job_id?: string;
  last_error?: string;
  updated_at?: string;
};

export type ClientAccessOverview = {
  client: ClientDetail;
  accesses: ClientServiceAccess[];
  groups: ClientAccessGroup[];
  vless_group?: ClientAccessGroup | null;
};

export type ClientAccessGroupAssignment = {
  client_id: string;
  group_id: string;
  service_code: string;
  mode: 'add_only' | 'add_or_move';
};

export type ClientAccessGroupAssignmentPreviewRequest = ClientAccessGroupAssignment & {
  build_artifacts?: boolean;
};

export type ClientAccessGroupAssignmentPreviewResult = ClientAccessGroupMembershipResult;

export type ClientAccessGroupAssignmentApplyRequest = ClientAccessGroupAssignment & {
  build_artifacts?: boolean;
};

export type ClientAccessGroupAssignmentApplyResult = ClientAccessGroupMembershipResult;

export type APIValidationError = {
  error?: string;
  field?: string;
  fields?: Record<string, string>;
  details?: unknown;
};

export type FirewallPolicy = APIRecord & {
  id: string;
  key?: string;
  label?: string;
  description?: string;
  scope?: string;
  node_id?: string;
  node_name?: string;
  default_input_policy?: string;
  default_forward_policy?: string;
  default_output_policy?: string;
  status?: string;
  rule_count?: number;
  created_at?: string;
  updated_at?: string;
};

export type FirewallRule = APIRecord & {
  id: string;
  policy_id?: string;
  priority?: number;
  chain?: 'input' | 'forward' | 'output' | string;
  action?: 'accept' | 'drop' | 'reject' | string;
  direction?: string;
  protocol?: string;
  src_list_id?: string;
  src_list_key?: string;
  dst_list_id?: string;
  dst_list_key?: string;
  src_cidr?: string;
  dst_cidr?: string;
  src_ports?: string;
  dst_ports?: string;
  state_match?: string[];
  comment?: string;
  enabled?: boolean;
  log?: boolean;
  status?: string;
  metadata?: APIRecord;
  created_at?: string;
  updated_at?: string;
};

export type FirewallAddressGroup = APIRecord & {
  id: string;
  key?: string;
  label?: string;
  description?: string;
  scope?: string;
  status?: string;
  entry_count?: number;
  created_at?: string;
  updated_at?: string;
};

export type FirewallAddressGroupEntry = APIRecord & {
  id: string;
  list_id?: string;
  list_key?: string;
  value?: string;
  value_type?: 'address' | 'cidr' | 'range' | 'dns' | string;
  label?: string;
  status?: string;
  created_at?: string;
  updated_at?: string;
};

export type FirewallNodeState = APIRecord & {
  id?: string;
  node_id: string;
  node_name?: string;
  policy_id?: string;
  policy_key?: string;
  revision_id?: string;
  desired_revision_id?: string;
  status?: string;
  observed?: APIRecord;
  last_job_id?: string;
  updated_at?: string;
};

export type FirewallManagementSettings = {
  control_plane_source_cidrs?: string[];
  ssh_bootstrap_source_cidrs?: string[];
  trusted_operator_cidrs?: string[];
};

export type FirewallSafetyWarning = {
  code?: string;
  message: string;
  severity?: string;
};

export type FirewallSafetyError = {
  code?: string;
  message: string;
  field?: string;
};

export type FirewallRenderedSet = APIRecord & {
  firewall_payload_hash?: string;
  safety_mode?: string;
  policy_id?: string;
  policy_key?: string;
  revision_id?: string;
  revision_no?: number;
  default_input_policy?: string;
  default_forward_policy?: string;
  default_output_policy?: string;
  enforce_default_policy?: boolean;
  rules?: APIRecord[];
  address_lists?: APIRecord[];
  ssh_bootstrap_ports?: number[];
  node_requires_forward_preservation?: boolean;
};

export type FirewallJobResult = APIRecord & {
  rendered_hash?: string;
  rendered_nftables?: string;
  rendered_summary?: string;
  warnings?: string[];
  blocking_errors?: string[];
  ssh_bootstrap_preserved?: boolean;
  control_plane_egress_preserved?: boolean;
  forward_egress_preserved?: boolean;
};

export type FirewallPreviewRequest = {
  policy_id?: string;
  enforce_default_policy?: boolean;
};

export type FirewallPreviewResult = Job & {
  payload: FirewallRenderedSet;
  result: FirewallJobResult;
};

export type FirewallApplyRequest = {
  policy_id?: string;
  enforce_default_policy?: boolean;
};

export type FirewallApplyResult = Job & {
  payload: FirewallRenderedSet;
  result: FirewallJobResult;
};

export type FirewallDisableResult = Job & {
  result: FirewallJobResult;
};

export type FirewallInventory = {
  address_lists: FirewallAddressGroup[];
  entries: FirewallAddressGroupEntry[];
  policies: FirewallPolicy[];
  rules: FirewallRule[];
  node_states: FirewallNodeState[];
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

export type JobRef = Pick<Job, 'id' | 'type' | 'status'> & APIRecord;

export type Artifact = APIRecord & {
  id: string;
  client_account_id?: string;
  client_id?: string;
  artifact_type?: string;
  type?: string;
  status?: string;
  service_access_id?: string | null;
  storage_path?: string;
  path?: string;
  content_hash?: string;
  size_bytes?: number;
  created_at?: string;
};

export type ClientArtifact = Artifact;

export type ClientArtifactBuildRequest = {
  type?: string;
  instance_ids?: string[];
};

export type ClientArtifactBuildResult = APIRecord & {
  job: JobRef;
  requested_type?: string;
  message?: string;
};

export type ClientArtifactDownloadResult = {
  url: string;
  method: 'GET';
};

export type ClientArtifactDeleteResult = APIRecord & {
  client_id?: string;
  artifact_id?: string;
  artifact_type?: string;
  deleted?: boolean;
  share_links_deleted?: number;
};

export type ShareLink = APIRecord & {
  id: string;
  client_id?: string;
  client_account_id?: string;
  target_type?: string;
  target_id?: string;
  token?: string;
  status?: string;
  token_hint?: string;
  url?: string;
  expires_at?: string;
  download_count?: number;
  created_at?: string;
};

export type ClientShareLink = ShareLink;

export type ClientShareLinkCreateInput = {
  target_id: string;
  ttl_hours?: number;
};

export type ClientShareLinkCreateResult = {
  share_link: ClientShareLink;
  share_url?: string;
  one_time_secret?: OneTimeSecretDisplay;
};

export type ClientShareLinkRevokeResult = ClientShareLink;

export type ClientShareLinkRotateResult = ClientShareLinkCreateResult & {
  rotated_from?: string;
};

export type ClientSubscription = APIRecord & {
  id: string;
  client_account_id?: string;
  token_hint?: string;
  status?: string;
  expires_at?: string;
  download_count?: number;
  last_used_at?: string | null;
  created_at?: string;
  updated_at?: string;
};

export type ClientSubscriptionCreateInput = {
  ttl_hours?: number;
};

export type ClientSubscriptionCreateResult = APIRecord & {
  subscription: ClientSubscription;
  subscription_url?: string;
  message?: string;
  one_time_secret?: OneTimeSecretDisplay;
};

export type ClientSubscriptionRotateResult = ClientSubscriptionCreateResult;

export type ClientSubscriptionRevokeResult = ClientSubscription;

export type ClientEmailDeliveryInput = {
  subject?: string;
  message?: string;
  ttl_hours?: number;
  create_share_link?: boolean;
};

export type ClientEmailDelivery = APIRecord & {
  id: string;
  client_account_id?: string;
  email?: string;
  subject?: string;
  status?: string;
  artifact_ids?: string[];
  share_link_ids?: string[];
  payload?: APIRecord;
  error_text?: string;
  sent_at?: string | null;
  created_at?: string;
};

export type ClientEmailDeliveryResult = APIRecord & {
  status?: string;
  delivery?: ClientEmailDelivery;
  error?: string;
};

export type ClientDeliveryHistoryItem = ClientEmailDelivery;

export type OneTimeSecretDisplay = {
  kind: 'share_link' | 'subscription';
  label: string;
  value: string;
  object_id?: string;
  expires_at?: string;
};

export type DeliveryJobRef = JobRef;

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
