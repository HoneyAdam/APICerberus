import { useEffect, useMemo, useState } from "react";
import { MoonStar, Search, SunMedium } from "lucide-react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { useTheme } from "@/components/layout/ThemeProvider";
import { Badge } from "@/components/ui/badge";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from "@/components/ui/command";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { NAV_ITEMS } from "./navigation";

function toTitleCase(segment: string) {
  if (!segment) {
    return "";
  }
  return segment
    .split("-")
    .filter(Boolean)
    .map((part) => `${part.charAt(0).toUpperCase()}${part.slice(1)}`)
    .join(" ");
}

export function Header() {
  const navigate = useNavigate();
  const location = useLocation();
  const { resolvedMode, toggleMode } = useTheme();
  const [commandOpen, setCommandOpen] = useState(false);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key.toLowerCase() !== "k") {
        return;
      }
      if (!(event.ctrlKey || event.metaKey)) {
        return;
      }
      event.preventDefault();
      setCommandOpen((current) => !current);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
    };
  }, []);

  const breadcrumbs = useMemo(() => {
    if (location.pathname === "/") {
      return [{ label: "Dashboard", path: "/" }];
    }
    const segments = location.pathname.split("/").filter(Boolean);
    const items: { label: string; path: string }[] = [];
    let cursor = "";
    for (const segment of segments) {
      cursor += `/${segment}`;
      const match = NAV_ITEMS.find((item) => item.path === cursor);
      items.push({
        label: match?.title ?? toTitleCase(segment),
        path: cursor,
      });
    }
    return items;
  }, [location.pathname]);

  return (
    <>
      <header className="sticky top-0 z-20 border-b border-border/80 bg-background/90 backdrop-blur-sm">
        <div className="flex h-14 items-center gap-2 px-3 md:px-6">
          <SidebarTrigger />
          <Separator orientation="vertical" className="mx-1 h-5" />

          <Breadcrumb className="hidden md:block">
            <BreadcrumbList>
              {breadcrumbs.map((item, index) => {
                const last = index === breadcrumbs.length - 1;
                return (
                  <BreadcrumbItem key={item.path}>
                    {last ? (
                      <BreadcrumbPage>{item.label}</BreadcrumbPage>
                    ) : (
                      <BreadcrumbLink asChild>
                        <Link to={item.path}>{item.label}</Link>
                      </BreadcrumbLink>
                    )}
                    {!last && <BreadcrumbSeparator />}
                  </BreadcrumbItem>
                );
              })}
            </BreadcrumbList>
          </Breadcrumb>

          <div className="ml-auto flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className="hidden min-w-40 justify-start text-muted-foreground lg:inline-flex"
              onClick={() => setCommandOpen(true)}
            >
              <Search className="mr-2 size-4" />
              Search
              <kbd className="ml-auto rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">Ctrl K</kbd>
            </Button>
            <Button variant="outline" size="icon" onClick={toggleMode} aria-label="Toggle theme">
              {resolvedMode === "dark" ? <SunMedium className="size-4" /> : <MoonStar className="size-4" />}
            </Button>
            <Badge variant="secondary" className="hidden sm:inline-flex">
              admin@local
            </Badge>
          </div>
        </div>
      </header>

      <CommandDialog open={commandOpen} onOpenChange={setCommandOpen}>
        <CommandInput placeholder="Go to page..." />
        <CommandList>
          <CommandEmpty>No route found.</CommandEmpty>
          <CommandGroup heading="Navigation">
            {NAV_ITEMS.map((item) => {
              const Icon = item.icon;
              return (
                <CommandItem
                  key={item.path}
                  value={`${item.title} ${item.description}`}
                  onSelect={() => {
                    navigate(item.path);
                    setCommandOpen(false);
                  }}
                >
                  <Icon className="size-4" />
                  <span>{item.title}</span>
                  <CommandShortcut>↵</CommandShortcut>
                </CommandItem>
              );
            })}
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </>
  );
}
