import { useEffect, useState, type PropsWithChildren } from "react";
import { useBreakpoint } from "@/hooks/use-media-query";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { AppSidebar } from "./AppSidebar";
import { Header } from "./Header";

export function AdminLayout({ children }: PropsWithChildren) {
  const isDesktop = useBreakpoint("md");
  const isLarge = useBreakpoint("lg");
  const [sidebarOpen, setSidebarOpen] = useState(true);

  useEffect(() => {
    if (!isDesktop) {
      return;
    }
    setSidebarOpen(isLarge);
  }, [isDesktop, isLarge]);

  return (
    <SidebarProvider open={sidebarOpen} onOpenChange={setSidebarOpen}>
      <AppSidebar />
      <SidebarInset>
        <Header />
        <section className="flex-1 p-4 md:p-6">{children}</section>
      </SidebarInset>
    </SidebarProvider>
  );
}

