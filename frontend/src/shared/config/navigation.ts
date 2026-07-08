import {
  Activity,
  Archive,
  Blocks,
  BriefcaseBusiness,
  Building2,
  ChartNoAxesCombined,
  CircleGauge,
  FileClock,
  Fingerprint,
  FlameKindling,
  GitBranch,
  Globe2,
  HardDriveDownload,
  KeyRound,
  Layers3,
  LockKeyhole,
  Mail,
  Map,
  Network,
  Route,
  ScrollText,
  Server,
  Settings,
  Shield,
  Users,
  Workflow,
} from 'lucide-react';
import type { ComponentType } from 'react';

export type NavItem = {
  id: string;
  labelKey: string;
  path: string;
  icon: ComponentType<{ size?: number; strokeWidth?: number }>;
  permission?: string;
};

export type NavSection = {
  id: string;
  labelKey: string;
  icon: ComponentType<{ size?: number; strokeWidth?: number }>;
  items: NavItem[];
};

export const navSections: NavSection[] = [
  {
    id: 'dashboard',
    labelKey: 'nav.dashboard',
    icon: CircleGauge,
    items: [{ id: 'dashboard', labelKey: 'nav.dashboard', path: '/', icon: CircleGauge, permission: 'dashboard.read' }],
  },
  {
    id: 'infrastructure',
    labelKey: 'nav.infrastructure',
    icon: Network,
    items: [
      { id: 'nodes', labelKey: 'nav.nodes', path: '/infrastructure/nodes', icon: Server, permission: 'node.read' },
      { id: 'node-map', labelKey: 'nav.nodeMap', path: '/infrastructure/node-map', icon: Map, permission: 'node.read' },
      { id: 'backhaul', labelKey: 'nav.backhaul', path: '/infrastructure/backhaul', icon: GitBranch, permission: 'node.read' },
      { id: 'address-pools', labelKey: 'nav.addressPools', path: '/infrastructure/address-pools', icon: Blocks, permission: 'instance.read' },
    ],
  },
  {
    id: 'services',
    labelKey: 'nav.services',
    icon: Layers3,
    items: [
      { id: 'instances', labelKey: 'nav.instances', path: '/services/instances', icon: Layers3, permission: 'instance.read' },
      { id: 'service-packs', labelKey: 'nav.servicePacks', path: '/services/service-packs', icon: BriefcaseBusiness, permission: 'service.read' },
      { id: 'runtime-artifacts', labelKey: 'nav.runtimeArtifacts', path: '/services/runtime-artifacts', icon: HardDriveDownload, permission: 'binary_repository.read' },
      { id: 'revisions', labelKey: 'nav.revisions', path: '/services/revisions', icon: FileClock, permission: 'instance.read' },
    ],
  },
  {
    id: 'clients',
    labelKey: 'nav.clientsRoot',
    icon: Users,
    items: [
      { id: 'clients', labelKey: 'nav.clients', path: '/clients', icon: Users, permission: 'client.read' },
      { id: 'groups', labelKey: 'nav.groups', path: '/clients/groups', icon: Workflow, permission: 'access_group.read' },
      { id: 'delivery', labelKey: 'nav.delivery', path: '/clients/delivery', icon: Archive, permission: 'artifact.read' },
      { id: 'subscriptions', labelKey: 'nav.subscriptions', path: '/clients/subscriptions', icon: KeyRound, permission: 'client.read' },
    ],
  },
  {
    id: 'network-policy',
    labelKey: 'nav.networkPolicy',
    icon: Shield,
    items: [
      { id: 'firewall', labelKey: 'nav.firewall', path: '/network-policy/firewall', icon: Shield, permission: 'firewall.read' },
      { id: 'route-policy', labelKey: 'nav.routePolicy', path: '/network-policy/route-policy', icon: Route, permission: 'node.read' },
      { id: 'traffic', labelKey: 'nav.traffic', path: '/network-policy/traffic', icon: ChartNoAxesCombined, permission: 'traffic.read' },
    ],
  },
  {
    id: 'operations',
    labelKey: 'nav.operations',
    icon: Activity,
    items: [
      { id: 'jobs', labelKey: 'nav.jobs', path: '/operations/jobs', icon: Activity, permission: 'job.read' },
      { id: 'audit', labelKey: 'nav.audit', path: '/operations/audit', icon: ScrollText, permission: 'audit.read' },
      { id: 'diagnostics', labelKey: 'nav.diagnostics', path: '/operations/diagnostics', icon: FlameKindling, permission: 'node.read' },
      { id: 'backup-restore', labelKey: 'nav.backupRestore', path: '/operations/backup-restore', icon: Building2, permission: 'settings.manage' },
    ],
  },
  {
    id: 'platform',
    labelKey: 'nav.platform',
    icon: Settings,
    items: [
      { id: 'settings', labelKey: 'nav.settings', path: '/platform/settings', icon: Settings, permission: 'settings.manage' },
      { id: 'certificates', labelKey: 'nav.certificates', path: '/platform/certificates', icon: LockKeyhole, permission: 'instance.read' },
      { id: 'access', labelKey: 'nav.access', path: '/platform/access', icon: Fingerprint, permission: 'auth.manage' },
      { id: 'mail', labelKey: 'nav.mail', path: '/platform/mail', icon: Mail, permission: 'auth.manage' },
      { id: 'legacy', labelKey: 'common.legacy', path: '/legacy/', icon: Globe2 },
    ],
  },
];
