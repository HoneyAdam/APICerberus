import { Activity, Shield, Zap } from "lucide-react";
import { Navigate, Route, Routes } from "react-router-dom";
import { AdminLayout } from "@/components/layout/AdminLayout";
import { ThemeProvider } from "@/components/layout/ThemeProvider";
import { NAV_ITEMS } from "@/components/layout/navigation";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ROUTES } from "@/lib/constants";

const DASHBOARD_CARDS = [
  { label: "Requests / min", value: "12,430", icon: Activity },
  { label: "Error Rate", value: "0.38%", icon: Zap },
  { label: "Protected Routes", value: "184", icon: Shield },
];

function DashboardPage() {
  return (
    <div className="page-shell">
      <header className="hero">
        <p className="eyebrow">API CERBERUS</p>
        <h1>Operational cockpit for secure API traffic.</h1>
        <p className="subtitle">
          Layout, responsive shell and shared dashboard primitives are ready for page-level CRUD and analytics screens.
        </p>
      </header>

      <main className="grid">
        {DASHBOARD_CARDS.map((card) => {
          const Icon = card.icon;
          return (
            <article key={card.label} className="stat-card">
              <div className="stat-top">
                <span>{card.label}</span>
                <Icon size={16} />
              </div>
              <strong>{card.value}</strong>
            </article>
          );
        })}
      </main>
    </div>
  );
}

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

export function App() {
  return (
    <ThemeProvider>
      <AdminLayout>
        <Routes>
          <Route path={ROUTES.dashboard} element={<DashboardPage />} />
          {NAV_ITEMS.filter((item) => item.path !== ROUTES.dashboard).map((item) => (
            <Route
              key={item.path}
              path={item.path}
              element={<PlaceholderPage title={item.title} description={item.description} />}
            />
          ))}
          <Route path="*" element={<Navigate to={ROUTES.dashboard} replace />} />
        </Routes>
      </AdminLayout>
    </ThemeProvider>
  );
}
