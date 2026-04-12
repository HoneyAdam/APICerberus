import { Suspense, lazy, useEffect, useState } from "react";
import { Navigate, Outlet, Route, Routes, useLocation, useSearchParams } from "react-router-dom";
import { AdminLayout } from "@/components/layout/AdminLayout";
import { PortalLayout } from "@/components/layout/PortalLayout";
import { ThemeProvider } from "@/components/layout/ThemeProvider";
import { BrandingProvider } from "@/components/layout/BrandingProvider";
import { NAV_ITEMS } from "@/components/layout/navigation";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { usePortalMe } from "@/hooks/use-portal";
import { ROUTES } from "@/lib/constants";
import { isAdminAuthenticated, setAdminAuthenticated } from "@/lib/api";
import { PORTAL_ROUTES } from "@/lib/portal-routes";
import { DashboardPage } from "@/pages/admin/Dashboard";
import { AdminLoginPage } from "@/pages/admin/Login";
import { PluginsPage } from "@/pages/admin/Plugins";
import { RoutesPage } from "@/pages/admin/Routes";
import { ServicesPage } from "@/pages/admin/Services";
import { UpstreamsPage } from "@/pages/admin/Upstreams";
import { ConsumersPage } from "@/pages/admin/Consumers";
import { UsersPage } from "@/pages/admin/Users";
import { AuditLogsPage } from "@/pages/admin/AuditLogs";
import { WelcomeModal, QuickSetupWizard, TourTooltip, DEFAULT_TOUR_STEPS, useTour } from "@/components/onboarding";

const LazyFallback = () => (
  <div className="flex items-center justify-center h-full min-h-[200px] text-sm text-muted-foreground">
    Loading...
  </div>
);

function Suspended({ children }: { children: React.ReactNode }) {
  return <Suspense fallback={<LazyFallback />}>{children}</Suspense>;
}

// Lazy-loaded pages (heavy dependencies: React Flow, CodeMirror, Recharts)
const AnalyticsPage = lazy(() => import("@/pages/admin/Analytics").then((m) => ({ default: m.AnalyticsPage })));
const AlertsPage = lazy(() => import("@/pages/admin/Alerts").then((m) => ({ default: m.AlertsPage })));
const AuditLogDetailPage = lazy(() => import("@/pages/admin/AuditLogDetail").then((m) => ({ default: m.AuditLogDetailPage })));
const ClusterPage = lazy(() => import("@/pages/admin/Cluster").then((m) => ({ default: m.ClusterPage })));
const ConfigPage = lazy(() => import("@/pages/admin/Config").then((m) => ({ default: m.ConfigPage })));
const CreditsPage = lazy(() => import("@/pages/admin/Credits").then((m) => ({ default: m.CreditsPage })));
const PluginMarketplacePage = lazy(() => import("@/pages/admin/PluginMarketplace").then((m) => ({ default: m.PluginMarketplacePage })));
const RouteBuilderPage = lazy(() => import("@/pages/admin/RouteBuilder").then((m) => ({ default: m.RouteBuilderPage })));
const RouteDetailPage = lazy(() => import("@/pages/admin/RouteDetail").then((m) => ({ default: m.RouteDetailPage })));
const ServiceDetailPage = lazy(() => import("@/pages/admin/ServiceDetail").then((m) => ({ default: m.ServiceDetailPage })));
const SettingsPage = lazy(() => import("@/pages/admin/Settings").then((m) => ({ default: m.SettingsPage })));
const SystemLogsPage = lazy(() => import("@/pages/admin/SystemLogs").then((m) => ({ default: m.SystemLogsPage })));
const UpstreamDetailPage = lazy(() => import("@/pages/admin/UpstreamDetail").then((m) => ({ default: m.UpstreamDetailPage })));
const UserDetailPage = lazy(() => import("@/pages/admin/UserDetail").then((m) => ({ default: m.UserDetailPage })));
const PortalAPIKeysPage = lazy(() => import("@/pages/portal/APIKeys").then((m) => ({ default: m.PortalAPIKeysPage })));
const PortalAPIsPage = lazy(() => import("@/pages/portal/APIs").then((m) => ({ default: m.PortalAPIsPage })));
const PortalCreditsPage = lazy(() => import("@/pages/portal/Credits").then((m) => ({ default: m.PortalCreditsPage })));
const PortalDashboardPage = lazy(() => import("@/pages/portal/Dashboard").then((m) => ({ default: m.PortalDashboardPage })));
const PortalLoginPage = lazy(() => import("@/pages/portal/Login").then((m) => ({ default: m.PortalLoginPage })));
const PortalLogDetailPage = lazy(() => import("@/pages/portal/LogDetail").then((m) => ({ default: m.PortalLogDetailPage })));
const PortalLogsPage = lazy(() => import("@/pages/portal/Logs").then((m) => ({ default: m.PortalLogsPage })));
const PortalPlaygroundPage = lazy(() => import("@/pages/portal/Playground").then((m) => ({ default: m.PortalPlaygroundPage })));
const PortalSecurityPage = lazy(() => import("@/pages/portal/Security").then((m) => ({ default: m.PortalSecurityPage })));
const PortalSettingsPage = lazy(() => import("@/pages/portal/Settings").then((m) => ({ default: m.PortalSettingsPage })));
const PortalUsagePage = lazy(() => import("@/pages/portal/Usage").then((m) => ({ default: m.PortalUsagePage })));

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
      <Route path={PORTAL_ROUTES.login} element={<Suspended><PortalLoginPage /></Suspended>} />

      <Route element={<RequirePortalSession />}>
        <Route element={<PortalLayout />}>
          <Route path={PORTAL_ROUTES.base} element={<Navigate to={PORTAL_ROUTES.dashboard} replace />} />
          <Route path={PORTAL_ROUTES.dashboard} element={<Suspended><PortalDashboardPage /></Suspended>} />
          <Route path={PORTAL_ROUTES.apiKeys} element={<Suspended><PortalAPIKeysPage /></Suspended>} />
          <Route path={PORTAL_ROUTES.apis} element={<Suspended><PortalAPIsPage /></Suspended>} />
          <Route path={PORTAL_ROUTES.playground} element={<Suspended><PortalPlaygroundPage /></Suspended>} />
          <Route path={PORTAL_ROUTES.usage} element={<Suspended><PortalUsagePage /></Suspended>} />
          <Route path={PORTAL_ROUTES.logs} element={<Suspended><PortalLogsPage /></Suspended>} />
          <Route path="/portal/logs/:id" element={<Suspended><PortalLogDetailPage /></Suspended>} />
          <Route path={PORTAL_ROUTES.credits} element={<Suspended><PortalCreditsPage /></Suspended>} />
          <Route path={PORTAL_ROUTES.security} element={<Suspended><PortalSecurityPage /></Suspended>} />
          <Route path={PORTAL_ROUTES.settings} element={<Suspended><PortalSettingsPage /></Suspended>} />
        </Route>
      </Route>

      <Route path="*" element={<Navigate to={PORTAL_ROUTES.login} replace />} />
    </Routes>
  );
}

function RequireAdminAuth() {
  const [searchParams] = useSearchParams();

  // Handle server-side login success - set sessionStorage before auth check
  if (searchParams.get("login") === "success") {
    setAdminAuthenticated(true);
    // Clean up URL immediately
    const url = new URL(window.location.href);
    url.searchParams.delete("login");
    window.history.replaceState({}, "", url.pathname + url.search);
  }

  return isAdminAuthenticated() ? <Outlet /> : <Navigate to="/login" replace />;
}

function AdminShell() {
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
    <>
      <Outlet />

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
    </>
  );
}

function AdminRoutesView() {
  return (
    <Routes>
      <Route path="/login" element={<AdminLoginPage />} />

      <Route element={<RequireAdminAuth />}>
        <Route element={<AdminLayout />}>
          <Route element={<AdminShell />}>
            <Route path={ROUTES.dashboard} element={<DashboardPage />} />
            <Route path={ROUTES.services} element={<ServicesPage />} />
            <Route path="/services/:id" element={<Suspended><ServiceDetailPage /></Suspended>} />
            <Route path={ROUTES.routes} element={<RoutesPage />} />
            <Route path="/routes/builder" element={<Suspended><RouteBuilderPage /></Suspended>} />
            <Route path="/routes/:id" element={<Suspended><RouteDetailPage /></Suspended>} />
            <Route path={ROUTES.upstreams} element={<UpstreamsPage />} />
            <Route path="/upstreams/:id" element={<Suspended><UpstreamDetailPage /></Suspended>} />
            <Route path={ROUTES.consumers} element={<ConsumersPage />} />
            <Route path={ROUTES.plugins} element={<PluginsPage />} />
            <Route path="/plugins/marketplace" element={<Suspended><PluginMarketplacePage /></Suspended>} />
            <Route path={ROUTES.users} element={<UsersPage />} />
            <Route path="/users/:id" element={<Suspended><UserDetailPage /></Suspended>} />
            <Route path={ROUTES.credits} element={<Suspended><CreditsPage /></Suspended>} />
            <Route path={ROUTES.auditLogs} element={<AuditLogsPage />} />
            <Route path="/audit-logs/:id" element={<Suspended><AuditLogDetailPage /></Suspended>} />
            <Route path={ROUTES.analytics} element={<Suspended><AnalyticsPage /></Suspended>} />
            <Route path={ROUTES.alerts} element={<Suspended><AlertsPage /></Suspended>} />
            <Route path={ROUTES.cluster} element={<Suspended><ClusterPage /></Suspended>} />
            <Route path={ROUTES.config} element={<Suspended><ConfigPage /></Suspended>} />
            <Route path={ROUTES.settings} element={<Suspended><SettingsPage /></Suspended>} />
            <Route path="/system-logs" element={<Suspended><SystemLogsPage /></Suspended>} />
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
          </Route>
        </Route>
      </Route>
    </Routes>
  );
}

export function App() {
  const location = useLocation();
  const portalMode = location.pathname === PORTAL_ROUTES.base || location.pathname.startsWith(`${PORTAL_ROUTES.base}/`);

  return <BrandingProvider><ThemeProvider>{portalMode ? <PortalRoutesView /> : <AdminRoutesView />}</ThemeProvider></BrandingProvider>;
}