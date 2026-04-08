import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import type { ColumnDef } from "@tanstack/react-table";
import { Plus, Wand2 } from "lucide-react";
import type { Route } from "@/lib/types";
import { ROUTES } from "@/lib/constants";
import { DataTable } from "@/components/shared/DataTable";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Link } from "react-router-dom";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useCreateRoute, useRoutes } from "@/hooks/use-routes";
import { useServices } from "@/hooks/use-services";

const ROUTE_COLUMNS: ColumnDef<Route>[] = [
  {
    accessorKey: "name",
    header: "Route",
    cell: ({ row }) => <span className="font-medium">{row.original.name}</span>,
  },
  {
    accessorKey: "service",
    header: "Service",
  },
  {
    id: "methods",
    header: "Methods",
    cell: ({ row }) => (
      <div className="flex flex-wrap gap-1">
        {row.original.methods.map((method) => (
          <Badge key={method} variant="outline">
            {method}
          </Badge>
        ))}
      </div>
    ),
  },
  {
    id: "plugins",
    header: "Plugins",
    cell: ({ row }) => (
      <div className="flex flex-wrap gap-1">
        {(row.original.plugins ?? []).slice(0, 3).map((plugin) => (
          <Badge key={plugin.name} variant="secondary">
            {plugin.name}
          </Badge>
        ))}
        {(row.original.plugins?.length ?? 0) > 3 ? (
          <Badge variant="outline">+{(row.original.plugins?.length ?? 0) - 3}</Badge>
        ) : null}
      </div>
    ),
  },
];

export function RoutesPage() {
  const navigate = useNavigate();
  const routesQuery = useRoutes();
  const servicesQuery = useServices();
  const createRoute = useCreateRoute();

  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [service, setService] = useState("");
  const [path, setPath] = useState("/new-route");
  const [method, setMethod] = useState("GET");

  const routeRows = useMemo(() => routesQuery.data ?? [], [routesQuery.data]);
  const services = useMemo(() => servicesQuery.data ?? [], [servicesQuery.data]);

  const handleCreate = async () => {
    if (!name.trim() || !service.trim() || !path.trim()) {
      return;
    }
    const created = await createRoute.mutateAsync({
      name: name.trim(),
      service: service.trim(),
      paths: [path.trim()],
      methods: [method],
    });
    setOpen(false);
    setName("");
    setPath("/new-route");
    setMethod("GET");
    navigate(ROUTES.routeDetail(created.id));
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-semibold">Routes</h2>
          <p className="text-sm text-muted-foreground">Define path matching and bind plugin chain behavior.</p>
        </div>

        <div className="flex gap-2">
          <Button variant="outline" asChild>
            <Link to="/routes/builder">
              <Wand2 className="mr-2 size-4" />
              Route Builder
            </Link>
          </Button>

        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button>
              <Plus className="mr-2 size-4" />
              New Route
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Create Route</DialogTitle>
              <DialogDescription>Set route name, bound service and primary path/method.</DialogDescription>
            </DialogHeader>
            <div className="space-y-3">
              <div className="space-y-1.5">
                <Label htmlFor="route-name">Name</Label>
                <Input id="route-name" value={name} onChange={(event) => setName(event.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label>Service</Label>
                <Select value={service} onValueChange={setService}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select a service" />
                  </SelectTrigger>
                  <SelectContent>
                    {services.map((svc) => (
                      <SelectItem key={svc.id} value={svc.id}>
                        {svc.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="route-path">Path</Label>
                <Input id="route-path" value={path} onChange={(event) => setPath(event.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label>Method</Label>
                <Select value={method} onValueChange={setMethod}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {["GET", "POST", "PUT", "PATCH", "DELETE"].map((verb) => (
                      <SelectItem key={verb} value={verb}>
                        {verb}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button onClick={handleCreate} disabled={createRoute.isPending}>
                Create
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
        </div>
      </div>

      <DataTable<Route, unknown>
        data={routeRows}
        columns={ROUTE_COLUMNS}
        searchColumn="name"
        searchPlaceholder="Search route..."
        fileName="routes"
        className="rounded-lg border bg-card p-3"
      />
    </div>
  );
}
