import { useEffect, useMemo, useState } from "react";
import { Search, Menu, Command } from "lucide-react";
import { Link, useLocation, useNavigate } from "react-router-dom";
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
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { ThemeToggle } from "./ThemeToggle";
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
  const [commandOpen, setCommandOpen] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

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
          <Separator orientation="vertical" className="mx-1 h-5 hidden sm:block" />

          {/* Mobile Breadcrumb - Show only last item */}
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

          {/* Mobile: Show current page title */}
          <div className="md:hidden flex-1 min-w-0">
            <span className="font-medium truncate">
              {breadcrumbs[breadcrumbs.length - 1]?.label}
            </span>
          </div>

          <div className="ml-auto flex items-center gap-2">
            {/* Mobile Search Button */}
            <Button
              variant="outline"
              size="icon"
              className="lg:hidden"
              onClick={() => setCommandOpen(true)}
              aria-label="Search"
            >
              <Search className="size-4" />
            </Button>

            {/* Desktop Search */}
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

            {/* Mobile Menu */}
            <Sheet open={mobileMenuOpen} onOpenChange={setMobileMenuOpen}>
              <SheetTrigger asChild>
                <Button variant="outline" size="icon" className="sm:hidden" aria-label="Menu">
                  <Menu className="size-4" />
                </Button>
              </SheetTrigger>
              <SheetContent side="right" className="w-[280px] sm:w-[350px]">
                <SheetHeader>
                  <SheetTitle>Menu</SheetTitle>
                </SheetHeader>
                <div className="mt-6 space-y-4">
                  <div className="space-y-2">
                    <p className="text-sm font-medium text-muted-foreground">Quick Actions</p>
                    <Button
                      variant="outline"
                      className="w-full justify-start"
                      onClick={() => {
                        setCommandOpen(true);
                        setMobileMenuOpen(false);
                      }}
                    >
                      <Command className="mr-2 size-4" />
                      Command Palette
                      <kbd className="ml-auto rounded bg-muted px-1.5 py-0.5 text-[10px]">Ctrl K</kbd>
                    </Button>
                  </div>

                  <div className="pt-4 border-t">
                    <Badge variant="secondary" className="w-full justify-center">
                      admin@local
                    </Badge>
                  </div>
                </div>
              </SheetContent>
            </Sheet>

            {/* Desktop Theme Toggle - 3-way (Light/Dark/System) */}
            <ThemeToggle className="hidden sm:flex" />

            {/* Desktop User Badge */}
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
