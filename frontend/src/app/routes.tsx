import { lazy, Suspense } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { AuthGate } from '../features/auth/AuthPages';
import { AppShell } from '../shared/layout/AppShell';
import { useAuth } from '../shared/auth/AuthProvider';
import { LoadingSkeleton } from '../shared/ui';
import { RouteErrorBoundary } from './RouteErrorBoundary';

const DashboardPage = lazy(() => import('../pages/dashboard/DashboardPage').then((module) => ({ default: module.DashboardPage })));
const NodesPage = lazy(() => import('../pages/infrastructure/NodesPage').then((module) => ({ default: module.NodesPage })));
const NodeMapPage = lazy(() => import('../pages/infrastructure/NodeMapPage').then((module) => ({ default: module.NodeMapPage })));
const BackhaulPage = lazy(() => import('../pages/infrastructure/BackhaulPage').then((module) => ({ default: module.BackhaulPage })));
const ExternalEgressPage = lazy(() => import('../pages/infrastructure/ExternalEgressPage').then((module) => ({ default: module.ExternalEgressPage })));
const AddressPoolsPage = lazy(() => import('../pages/infrastructure/AddressPoolsPage').then((module) => ({ default: module.AddressPoolsPage })));
const InstancesPage = lazy(() => import('../pages/services/InstancesPage').then((module) => ({ default: module.InstancesPage })));
const ServicePacksPage = lazy(() => import('../pages/services/ServicePacksPage').then((module) => ({ default: module.ServicePacksPage })));
const RuntimeArtifactsPage = lazy(() => import('../pages/services/RuntimeArtifactsPage').then((module) => ({ default: module.RuntimeArtifactsPage })));
const ClientsPage = lazy(() => import('../pages/clients/ClientsPage').then((module) => ({ default: module.ClientsPage })));
const ClientGroupsPage = lazy(() => import('../pages/clients/ClientGroupsPage').then((module) => ({ default: module.ClientGroupsPage })));
const DeliveryPage = lazy(() => import('../pages/clients/DeliveryPage').then((module) => ({ default: module.DeliveryPage })));
const SubscriptionsPage = lazy(() => import('../pages/clients/SubscriptionsPage').then((module) => ({ default: module.SubscriptionsPage })));
const FirewallPage = lazy(() => import('../pages/network-policy/FirewallPage').then((module) => ({ default: module.FirewallPage })));
const RoutePolicyPage = lazy(() => import('../pages/network-policy/RoutePolicyPage').then((module) => ({ default: module.RoutePolicyPage })));
const TrafficPage = lazy(() => import('../pages/network-policy/TrafficPage').then((module) => ({ default: module.TrafficPage })));
const JobsPage = lazy(() => import('../pages/operations/JobsPage').then((module) => ({ default: module.JobsPage })));
const AuditPage = lazy(() => import('../pages/operations/AuditPage').then((module) => ({ default: module.AuditPage })));
const DiagnosticsPage = lazy(() => import('../pages/operations/DiagnosticsPage').then((module) => ({ default: module.DiagnosticsPage })));
const BackupRestorePage = lazy(() => import('../pages/operations/BackupRestorePage').then((module) => ({ default: module.BackupRestorePage })));
const SettingsPage = lazy(() => import('../pages/platform/SettingsPage').then((module) => ({ default: module.SettingsPage })));
const CertificatesPage = lazy(() => import('../pages/platform/CertificatesPage').then((module) => ({ default: module.CertificatesPage })));
const AccessPage = lazy(() => import('../pages/platform/AccessPage').then((module) => ({ default: module.AccessPage })));
const MailPage = lazy(() => import('../pages/platform/MailPage').then((module) => ({ default: module.MailPage })));

function ProtectedRoutes() {
  const auth = useAuth();
  if (auth.isLoading) {
    return <main className="auth-page"><LoadingSkeleton /></main>;
  }
  if (!auth.isAuthenticated) {
    return <Navigate to={`/auth${window.location.search || ''}`} replace />;
  }
  return <AppShell />;
}

export function AppRoutes() {
  return (
    <RouteErrorBoundary>
      <Suspense fallback={<main className="auth-page"><LoadingSkeleton /></main>}>
        <Routes>
        <Route path="/auth" element={<AuthGate />} />
        <Route element={<ProtectedRoutes />}>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/infrastructure/nodes" element={<NodesPage />} />
          <Route path="/infrastructure/node-map" element={<NodeMapPage />} />
          <Route path="/infrastructure/backhaul" element={<BackhaulPage />} />
          <Route path="/infrastructure/external-egress" element={<ExternalEgressPage />} />
          <Route path="/infrastructure/address-pools" element={<AddressPoolsPage />} />
          <Route path="/services/instances" element={<InstancesPage />} />
          <Route path="/services/service-packs" element={<ServicePacksPage />} />
          <Route path="/services/runtime-artifacts" element={<RuntimeArtifactsPage />} />
          <Route path="/services/revisions" element={<Navigate to="/services/instances" replace />} />
          <Route path="/clients" element={<ClientsPage />} />
          <Route path="/clients/groups" element={<ClientGroupsPage />} />
          <Route path="/clients/delivery" element={<DeliveryPage />} />
          <Route path="/clients/subscriptions" element={<SubscriptionsPage />} />
          <Route path="/network-policy/firewall" element={<FirewallPage />} />
          <Route path="/network-policy/route-policy" element={<RoutePolicyPage />} />
          <Route path="/network-policy/traffic" element={<TrafficPage />} />
          <Route path="/operations/jobs" element={<JobsPage />} />
          <Route path="/operations/audit" element={<AuditPage />} />
          <Route path="/operations/diagnostics" element={<DiagnosticsPage />} />
          <Route path="/operations/backup-restore" element={<BackupRestorePage />} />
          <Route path="/platform/settings" element={<SettingsPage />} />
          <Route path="/platform/certificates" element={<CertificatesPage />} />
          <Route path="/platform/access" element={<AccessPage />} />
          <Route path="/platform/mail" element={<MailPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Suspense>
    </RouteErrorBoundary>
  );
}
