import { useMemo, useState } from "react";
import { Search, Sparkles } from "lucide-react";
import type { PortalAPIItem } from "@/lib/portal-types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { newKVRow, type PlaygroundDraft, type PlaygroundKVRow } from "./types";

const HTTP_METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"] as const;

type RequestBuilderProps = {
  draft: PlaygroundDraft;
  apis: PortalAPIItem[];
  onChange: (updater: (current: PlaygroundDraft) => PlaygroundDraft) => void;
};

function resolveCostBadge(draft: PlaygroundDraft, apis: PortalAPIItem[]) {
  if (draft.selectedRouteID) {
    const matched = apis.find((item) => item.route_id === draft.selectedRouteID);
    if (matched) {
      return matched.credit_cost;
    }
  }
  const matchedByPath = apis.find((item) => item.paths.some((path) => path === draft.path));
  return matchedByPath?.credit_cost;
}

function upsertRows(rows: PlaygroundKVRow[], index: number, next: Partial<PlaygroundKVRow>) {
  return rows.map((row, rowIndex) => {
    if (rowIndex !== index) {
      return row;
    }
    return {
      ...row,
      ...next,
    };
  });
}

export function RequestBuilder({ draft, apis, onChange }: RequestBuilderProps) {
  const [commandOpen, setCommandOpen] = useState(false);

  const cost = useMemo(() => resolveCostBadge(draft, apis), [draft, apis]);

  return (
    <div className="space-y-3 rounded-xl border bg-background p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 className="text-sm font-semibold">Request Builder</h3>
          <p className="text-xs text-muted-foreground">Select method, path and query parameters.</p>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="outline" className="font-mono">
            Cost: {cost ?? "--"}
          </Badge>
          <Popover open={commandOpen} onOpenChange={setCommandOpen}>
            <PopoverTrigger asChild>
              <Button variant="outline" size="sm">
                <Search className="mr-2 size-4" />
                Route Finder
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-[420px] p-0" align="end">
              <Command>
                <CommandInput placeholder="Search route or path..." />
                <CommandList>
                  <CommandEmpty>No route found.</CommandEmpty>
                  <CommandGroup heading="Allowed APIs">
                    {apis.map((item) => (
                      <CommandItem
                        key={item.route_id}
                        value={`${item.route_name} ${item.paths.join(" ")} ${item.methods.join(" ")}`}
                        onSelect={() => {
                          onChange((current) => ({
                            ...current,
                            selectedRouteID: item.route_id,
                            path: item.paths[0] ?? current.path,
                            method: item.methods[0] ?? current.method,
                          }));
                          setCommandOpen(false);
                        }}
                      >
                        <Sparkles className="size-4" />
                        <div className="flex flex-col">
                          <span className="text-xs font-medium">{item.route_name}</span>
                          <span className="text-[11px] text-muted-foreground">{item.paths.join(", ")}</span>
                        </div>
                        <Badge variant="secondary" className="ml-auto">
                          {item.credit_cost}
                        </Badge>
                      </CommandItem>
                    ))}
                  </CommandGroup>
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>
        </div>
      </div>

      <div className="grid gap-3 md:grid-cols-[140px_1fr]">
        <div className="space-y-1.5">
          <Label>Method</Label>
          <Select
            value={draft.method}
            onValueChange={(method) =>
              onChange((current) => ({
                ...current,
                method,
              }))
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {HTTP_METHODS.map((method) => (
                <SelectItem key={method} value={method}>
                  {method}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-1.5">
          <Label>Path</Label>
          <Input
            value={draft.path}
            onChange={(event) =>
              onChange((current) => ({
                ...current,
                path: event.target.value,
              }))
            }
            placeholder="/v1/users"
          />
        </div>
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <Label>Query Parameters</Label>
          <Button
            size="sm"
            variant="outline"
            onClick={() =>
              onChange((current) => ({
                ...current,
                queryRows: [...current.queryRows, newKVRow()],
              }))
            }
          >
            Add Param
          </Button>
        </div>

        <div className="space-y-2">
          {draft.queryRows.map((row, index) => (
            <div key={row.id} className="grid gap-2 md:grid-cols-[1fr_1fr_auto]">
              <Input
                value={row.key}
                placeholder="key"
                onChange={(event) =>
                  onChange((current) => ({
                    ...current,
                    queryRows: upsertRows(current.queryRows, index, { key: event.target.value }),
                  }))
                }
              />
              <Input
                value={row.value}
                placeholder="value"
                onChange={(event) =>
                  onChange((current) => ({
                    ...current,
                    queryRows: upsertRows(current.queryRows, index, { value: event.target.value }),
                  }))
                }
              />
              <Button
                variant="ghost"
                onClick={() =>
                  onChange((current) => ({
                    ...current,
                    queryRows: current.queryRows.filter((item) => item.id !== row.id),
                  }))
                }
              >
                Remove
              </Button>
            </div>
          ))}
          {draft.queryRows.length === 0 ? (
            <p className="text-xs text-muted-foreground">No query params configured.</p>
          ) : null}
        </div>
      </div>
    </div>
  );
}
