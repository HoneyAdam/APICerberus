import { useEffect, useState, type PropsWithChildren } from "react";
import { useBreakpoint } from "@/hooks/use-media-query";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { AppSidebar } from "./AppSidebar";
import { Header } from "./Header";
import { useIsMobile } from "@/hooks/use-mobile";

export function AdminLayout({ children }: PropsWithChildren) {
  const isDesktop = useBreakpoint("md");
  const isLarge = useBreakpoint("lg");
  const isMobile = useIsMobile();
  const [sidebarOpen, setSidebarOpen] = useState(true);

  useEffect(() => {
    if (isMobile) {
      // On mobile, sidebar starts closed
      setSidebarOpen(false);
    } else if (!isDesktop) {
      // On tablet, sidebar starts closed
      setSidebarOpen(false);
    } else {
      // On desktop, sidebar follows large breakpoint
      setSidebarOpen(isLarge);
    }
  }, [isDesktop, isLarge, isMobile]);

  return (
    <SidebarProvider open={sidebarOpen} onOpenChange={setSidebarOpen}>
      <AppSidebar />
      <SidebarInset>
        <Header />
        <section className="flex-1 p-3 sm:p-4 md:p-6 overflow-x-auto">{children}</section>
      </SidebarInset>
    </SidebarProvider>
  );
}

