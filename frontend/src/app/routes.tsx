import { Navigate, Route, Routes } from 'react-router-dom';
import { AuthGate } from '../features/auth/AuthPages';
import { AppShell } from '../shared/layout/AppShell';
import { useAuth } from '../shared/auth/AuthProvider';
import { LoadingSkeleton } from '../shared/ui';
import { DashboardPage } from '../pages/dashboard/DashboardPage';
import { NodesPage } from '../pages/infrastructure/NodesPage';
import { NodeMapPage } from '../pages/infrastructure/NodeMapPage';
import { BackhaulPage } from '../pages/infrastructure/BackhaulPage';
import { AddressPoolsPage } from '../pages/infrastructure/AddressPoolsPage';
import { InstancesPage } from '../pages/services/InstancesPage';
import { ServicePacksPage } from '../pages/services/ServicePacksPage';
import { RuntimeArtifactsPage } from '../pages/services/RuntimeArtifactsPage';
import { RevisionsPage } from '../pages/services/RevisionsPage';
import { ClientsPage } from '../pages/clients/ClientsPage';
import { ClientGroupsPage } from '../pages/clients/ClientGroupsPage';
import { DeliveryPage } from '../pages/clients/DeliveryPage';
import { SubscriptionsPage } from '../pages/clients/SubscriptionsPage';
import { FirewallPage } from '../pages/network-policy/FirewallPage';
import { RoutePolicyPage } from '../pages/network-policy/RoutePolicyPage';
import { TrafficPage } from '../pages/network-policy/TrafficPage';
import { JobsPage } from '../pages/operations/JobsPage';
import { AuditPage } from '../pages/operations/AuditPage';
import { DiagnosticsPage } from '../pages/operations/DiagnosticsPage';
import { BackupRestorePage } from '../pages/operations/BackupRestorePage';
import { SettingsPage } from '../pages/platform/SettingsPage';
import { CertificatesPage } from '../pages/platform/CertificatesPage';
import { AccessPage } from '../pages/platform/AccessPage';
import { MailPage } from '../pages/platform/MailPage';

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
    <Routes>
      <Route path="/auth" element={<AuthGate />} />
      <Route element={<ProtectedRoutes />}>
        <Route path="/" element={<DashboardPage />} />
        <Route path="/infrastructure/nodes" element={<NodesPage />} />
        <Route path="/infrastructure/node-map" element={<NodeMapPage />} />
        <Route path="/infrastructure/backhaul" element={<BackhaulPage />} />
        <Route path="/infrastructure/address-pools" element={<AddressPoolsPage />} />
        <Route path="/services/instances" element={<InstancesPage />} />
        <Route path="/services/service-packs" element={<ServicePacksPage />} />
        <Route path="/services/runtime-artifacts" element={<RuntimeArtifactsPage />} />
        <Route path="/services/revisions" element={<RevisionsPage />} />
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
  );
}
