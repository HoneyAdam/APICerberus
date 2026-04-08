import { useEffect, useState } from "react";
import { Navigate, Outlet, Route, Routes, useLocation } from "react-router-dom";
import { AdminLayout } from "@/components/layout/AdminLayout";
import { PortalLayout } from "@/components/layout/PortalLayout";
import { ThemeProvider } from "@/components/layout/ThemeProvider";
import { NAV_ITEMS } from "@/components/layout/navigation";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { usePortalMe } from "@/hooks/use-portal";
import { ROUTES } from "@/lib/constants";
import { PORTAL_ROUTES } from "@/lib/portal-routes";
import { AnalyticsPage } from "@/pages/admin/Analytics";
import { AlertsPage } from "@/pages/admin/Alerts";
import { AuditLogDetailPage } from "@/pages/admin/AuditLogDetail";
import { AuditLogsPage } from "@/pages/admin/AuditLogs";
import { ClusterPage } from "@/pages/admin/Cluster";
import { ConfigPage } from "@/pages/admin/Config";
import { ConsumersPage } from "@/pages/admin/Consumers";
import { CreditsPage } from "@/pages/admin/Credits";
import { DashboardPage } from "@/pages/admin/Dashboard";
import { PluginMarketplacePage } from "@/pages/admin/PluginMarketplace";
import { PluginsPage } from "@/pages/admin/Plugins";
import { RouteBuilderPage } from "@/pages/admin/RouteBuilder";
import { RouteDetailPage } from "@/pages/admin/RouteDetail";
import { RoutesPage } from "@/pages/admin/Routes";
import { ServiceDetailPage } from "@/pages/admin/ServiceDetail";
import { ServicesPage } from "@/pages/admin/Services";
import { SettingsPage } from "@/pages/admin/Settings";
import { SystemLogsPage } from "@/pages/admin/SystemLogs";
import { UpstreamDetailPage } from "@/pages/admin/UpstreamDetail";
import { UpstreamsPage } from "@/pages/admin/Upstreams";
import { UserDetailPage } from "@/pages/admin/UserDetail";
import { UsersPage } from "@/pages/admin/Users";
import { PortalAPIKeysPage } from "@/pages/portal/APIKeys";
import { PortalAPIsPage } from "@/pages/portal/APIs";
import { PortalCreditsPage } from "@/pages/portal/Credits";
import { PortalDashboardPage } from "@/pages/portal/Dashboard";
import { PortalLogDetailPage } from "@/pages/portal/LogDetail";
import { PortalLoginPage } from "@/pages/portal/Login";
import { PortalLogsPage } from "@/pages/portal/Logs";
import { PortalPlaygroundPage } from "@/pages/portal/Playground";
import { PortalSecurityPage } from "@/pages/portal/Security";
import { PortalSettingsPage } from "@/pages/portal/Settings";
import { PortalUsagePage } from "@/pages/portal/Usage";
import { WelcomeModal, QuickSetupWizard, TourTooltip, DEFAULT_TOUR_STEPS, useTour } from "@/components/onboarding";

function PlaceholderPage({ title, description }: { title: string; description: string }) {
  return (
    <div className="mx-auto max-w-5xl">
      <Card>
        <CardHeader>
          <Badge className="w-fit" variant="secondary">
            In Progress
          </Badge>
          <CardTitle>{title}</CardTitle>
          <CardDescription>{description}</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            This screen is scaffolded and connected to app navigation. CRUD widgets and data hooks land in subsequent
            tasks.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}

function RequirePortalSession() {
  const location = useLocation();
  const meQuery = usePortalMe();

  if (meQuery.isLoading) {
    return <div className="p-8 text-sm text-muted-foreground">Checking session...</div>;
  }

  if (!meQuery.data?.user) {
    return <Navigate to={PORTAL_ROUTES.login} state={{ from: location }} replace />;
  }

  return <Outlet />;
}

function PortalRoutesView() {
  return (
    <Routes>
      <Route path={PORTAL_ROUTES.login} element={<PortalLoginPage />} />

      <Route element={<RequirePortalSession />}>
        <Route element={<PortalLayout />}>
          <Route path={PORTAL_ROUTES.base} element={<Navigate to={PORTAL_ROUTES.dashboard} replace />} />
          <Route path={PORTAL_ROUTES.dashboard} element={<PortalDashboardPage />} />
          <Route path={PORTAL_ROUTES.apiKeys} element={<PortalAPIKeysPage />} />
          <Route path={PORTAL_ROUTES.apis} element={<PortalAPIsPage />} />
          <Route path={PORTAL_ROUTES.playground} element={<PortalPlaygroundPage />} />
          <Route path={PORTAL_ROUTES.usage} element={<PortalUsagePage />} />
          <Route path={PORTAL_ROUTES.logs} element={<PortalLogsPage />} />
          <Route path="/portal/logs/:id" element={<PortalLogDetailPage />} />
          <Route path={PORTAL_ROUTES.credits} element={<PortalCreditsPage />} />
          <Route path={PORTAL_ROUTES.security} element={<PortalSecurityPage />} />
          <Route path={PORTAL_ROUTES.settings} element={<PortalSettingsPage />} />
        </Route>
      </Route>

      <Route path="*" element={<Navigate to={PORTAL_ROUTES.login} replace />} />
    </Routes>
  );
}

function AdminRoutesView() {
  const [showWelcome, setShowWelcome] = useState(false);
  const [showQuickSetup, setShowQuickSetup] = useState(false);
  const { isOpen: isTourOpen, startTour, closeTour } = useTour();

  useEffect(() => {
    const hasSeenWelcome = localStorage.getItem("apicerberus.welcome_shown");
    if (!hasSeenWelcome) {
      const timer = setTimeout(() => {
        setShowWelcome(true);
      }, 500);
      return () => clearTimeout(timer);
    }
  }, []);

  const handleStartTour = () => {
    startTour();
  };

  const handleStartSetup = () => {
    setShowQuickSetup(true);
  };

  return (
    <AdminLayout>
      <Routes>
        <Route path={ROUTES.dashboard} element={<DashboardPage />} />
        <Route path={ROUTES.services} element={<ServicesPage />} />
        <Route path="/services/:id" element={<ServiceDetailPage />} />
        <Route path={ROUTES.routes} element={<RoutesPage />} />
        <Route path="/routes/builder" element={<RouteBuilderPage />} />
        <Route path="/routes/:id" element={<RouteDetailPage />} />
        <Route path={ROUTES.upstreams} element={<UpstreamsPage />} />
        <Route path="/upstreams/:id" element={<UpstreamDetailPage />} />
        <Route path={ROUTES.consumers} element={<ConsumersPage />} />
        <Route path={ROUTES.plugins} element={<PluginsPage />} />
        <Route path="/plugins/marketplace" element={<PluginMarketplacePage />} />
        <Route path={ROUTES.users} element={<UsersPage />} />
        <Route path="/users/:id" element={<UserDetailPage />} />
        <Route path={ROUTES.credits} element={<CreditsPage />} />
        <Route path={ROUTES.auditLogs} element={<AuditLogsPage />} />
        <Route path="/audit-logs/:id" element={<AuditLogDetailPage />} />
        <Route path={ROUTES.analytics} element={<AnalyticsPage />} />
        <Route path={ROUTES.alerts} element={<AlertsPage />} />
        <Route path={ROUTES.cluster} element={<ClusterPage />} />
        <Route path={ROUTES.config} element={<ConfigPage />} />
        <Route path={ROUTES.settings} element={<SettingsPage />} />
        <Route path="/system-logs" element={<SystemLogsPage />} />
        {NAV_ITEMS
          .filter(
            (item) =>
              item.path !== ROUTES.dashboard &&
              item.path !== ROUTES.services &&
              item.path !== ROUTES.routes &&
              item.path !== ROUTES.upstreams &&
              item.path !== ROUTES.consumers &&
              item.path !== ROUTES.plugins &&
              item.path !== ROUTES.users &&
              item.path !== ROUTES.credits &&
              item.path !== ROUTES.auditLogs &&
              item.path !== ROUTES.analytics &&
              item.path !== ROUTES.alerts &&
              item.path !== ROUTES.cluster &&
              item.path !== ROUTES.config &&
              item.path !== ROUTES.settings,
          )
          .map((item) => (
            <Route
              key={item.path}
              path={item.path}
              element={<PlaceholderPage title={item.title} description={item.description} />}
            />
          ))}
        <Route path="*" element={<Navigate to={ROUTES.dashboard} replace />} />
      </Routes>

      {/* Onboarding Components */}
      <WelcomeModal
        open={showWelcome}
        onOpenChange={setShowWelcome}
        onStartTour={handleStartTour}
        onStartSetup={handleStartSetup}
      />

      <QuickSetupWizard
        open={showQuickSetup}
        onOpenChange={setShowQuickSetup}
      />

      <TourTooltip
        steps={DEFAULT_TOUR_STEPS}
        isOpen={isTourOpen}
        onClose={closeTour}
      />
    </AdminLayout>
  );
}

export function App() {
  const location = useLocation();
  const portalMode = location.pathname === PORTAL_ROUTES.base || location.pathname.startsWith(`${PORTAL_ROUTES.base}/`);

  return <ThemeProvider>{portalMode ? <PortalRoutesView /> : <AdminRoutesView />}</ThemeProvider>;
}
