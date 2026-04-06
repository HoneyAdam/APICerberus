import { useState } from "react";

import { adminApiRequest } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Download, FileJson, FileSpreadsheet, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";

export type ExportEntity = "routes" | "services" | "upstreams" | "consumers" | "plugins" | "users" | "audit-logs";

export type ExportFormat = "json" | "csv" | "yaml";

type BulkExportProps = {
  className?: string;
};

const EXPORT_ENTITIES: { id: ExportEntity; label: string; description: string }[] = [
  { id: "routes", label: "Routes", description: "All route configurations" },
  { id: "services", label: "Services", description: "Service definitions" },
  { id: "upstreams", label: "Upstreams", description: "Upstream targets and health checks" },
  { id: "consumers", label: "Consumers", description: "Consumer credentials and metadata" },
  { id: "plugins", label: "Plugins", description: "Plugin configurations" },
  { id: "users", label: "Users", description: "Portal user accounts" },
  { id: "audit-logs", label: "Audit Logs", description: "Recent audit log entries" },
];

const ENTITY_ENDPOINTS: Record<ExportEntity, string> = {
  routes: "/admin/api/v1/routes",
  services: "/admin/api/v1/services",
  upstreams: "/admin/api/v1/upstreams",
  consumers: "/admin/api/v1/consumers",
  plugins: "/admin/api/v1/plugins",
  users: "/admin/api/v1/users",
  "audit-logs": "/admin/api/v1/audit-logs",
};

export function BulkExport({ className }: BulkExportProps) {
  const [selectedEntities, setSelectedEntities] = useState<ExportEntity[]>([]);
  const [format, setFormat] = useState<ExportFormat>("json");
  const [isExporting, setIsExporting] = useState(false);

  const toggleEntity = (entity: ExportEntity) => {
    setSelectedEntities((prev) =>
      prev.includes(entity)
        ? prev.filter((e) => e !== entity)
        : [...prev, entity]
    );
  };

  const selectAll = () => {
    setSelectedEntities(EXPORT_ENTITIES.map((e) => e.id));
  };

  const deselectAll = () => {
    setSelectedEntities([]);
  };

  const handleExport = async () => {
    if (selectedEntities.length === 0) return;

    setIsExporting(true);
    try {
      const exportData: Record<string, unknown> = {};

      for (const entity of selectedEntities) {
        try {
          const response = await adminApiRequest<Record<string, unknown> | unknown[]>(
            ENTITY_ENDPOINTS[entity],
            { query: { limit: 10000 } }
          );
          exportData[entity] = response;
        } catch (error) {
          console.error(`Failed to export ${entity}:`, error);
          exportData[entity] = { error: "Failed to fetch data" };
        }
      }

      // Generate and download file
      const timestamp = new Date().toISOString().replace(/[:.]/g, "-");
      const fileName = `apicerberus-export-${timestamp}`;

      if (format === "json") {
        downloadJSON(exportData, `${fileName}.json`);
      } else if (format === "csv") {
        downloadCSV(exportData, fileName);
      } else if (format === "yaml") {
        downloadYAML(exportData, `${fileName}.yaml`);
      }
    } finally {
      setIsExporting(false);
    }
  };

  const downloadJSON = (data: unknown, fileName: string) => {
    const blob = new Blob([JSON.stringify(data, null, 2)], {
      type: "application/json",
    });
    downloadBlob(blob, fileName);
  };

  const downloadCSV = (data: Record<string, unknown>, baseFileName: string) => {
    // Create a ZIP-like experience by downloading multiple CSVs
    Object.entries(data).forEach(([entity, items]) => {
      if (Array.isArray(items) && items.length > 0) {
        const csv = convertToCSV(items);
        const blob = new Blob([csv], { type: "text/csv" });
        downloadBlob(blob, `${baseFileName}-${entity}.csv`);
      }
    });
  };

  const downloadYAML = (data: unknown, fileName: string) => {
    // Simple YAML conversion (in production, use a proper YAML library)
    const yaml = convertToYAML(data);
    const blob = new Blob([yaml], { type: "application/yaml" });
    downloadBlob(blob, fileName);
  };

  const downloadBlob = (blob: Blob, fileName: string) => {
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = fileName;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
  };

  const convertToCSV = (items: unknown[]): string => {
    if (items.length === 0) return "";

    const headers = Object.keys(items[0] as Record<string, unknown>);
    const rows = items.map((item) =>
      headers.map((header) => {
        const value = (item as Record<string, unknown>)[header];
        const stringValue = typeof value === "object" ? JSON.stringify(value) : String(value ?? "");
        // Escape quotes and wrap in quotes if contains comma
        if (stringValue.includes(",") || stringValue.includes('"')) {
          return `"${stringValue.replace(/"/g, '""')}"`;
        }
        return stringValue;
      })
    );

    return [headers.join(","), ...rows.map((r) => r.join(","))].join("\n");
  };

  const convertToYAML = (data: unknown, indent = 0): string => {
    const spaces = "  ".repeat(indent);

    if (data === null || data === undefined) {
      return "null";
    }

    if (typeof data === "string") {
      if (data.includes("\n") || data.includes('"')) {
        return `|${data.includes("\n\n") ? "2" : ""}\n${spaces}  ${data.replace(/\n/g, `\n${spaces}  `)}`;
      }
      return data;
    }

    if (typeof data === "number" || typeof data === "boolean") {
      return String(data);
    }

    if (Array.isArray(data)) {
      if (data.length === 0) return "[]";
      return data.map((item) => `${spaces}- ${convertToYAML(item, indent + 1).trimStart()}`).join("\n");
    }

    if (typeof data === "object") {
      const entries = Object.entries(data);
      if (entries.length === 0) return "{}";
      return entries
        .map(([key, value]) => {
          const yamlValue = convertToYAML(value, indent + 1);
          if (typeof value === "object" && value !== null && !Array.isArray(value)) {
            return `${spaces}${key}:\n${yamlValue}`;
          }
          return `${spaces}${key}: ${yamlValue}`;
        })
        .join("\n");
    }

    return String(data);
  };

  return (
    <Card className={className}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Download className="h-5 w-5" />
          Bulk Export
        </CardTitle>
        <CardDescription>
          Export configuration and data for backup or migration
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-6">
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h4 className="text-sm font-medium">Select Entities to Export</h4>
            <div className="flex gap-2">
              <Button variant="ghost" size="sm" onClick={selectAll}>
                Select All
              </Button>
              <Button variant="ghost" size="sm" onClick={deselectAll}>
                Deselect All
              </Button>
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            {EXPORT_ENTITIES.map((entity) => (
              <div
                key={entity.id}
                className={cn(
                  "flex items-start space-x-3 rounded-lg border p-3 transition-colors",
                  selectedEntities.includes(entity.id) && "border-primary bg-primary/5"
                )}
              >
                <Checkbox
                  id={`export-${entity.id}`}
                  checked={selectedEntities.includes(entity.id)}
                  onCheckedChange={() => toggleEntity(entity.id)}
                />
                <div className="flex-1">
                  <Label
                    htmlFor={`export-${entity.id}`}
                    className="cursor-pointer font-medium"
                  >
                    {entity.label}
                  </Label>
                  <p className="text-xs text-muted-foreground">
                    {entity.description}
                  </p>
                </div>
              </div>
            ))}
          </div>
        </div>

        <Separator />

        <div className="space-y-4">
          <h4 className="text-sm font-medium">Export Format</h4>
          <div className="flex gap-2">
            <Button
              variant={format === "json" ? "default" : "outline"}
              size="sm"
              onClick={() => setFormat("json")}
            >
              <FileJson className="h-4 w-4 mr-1" />
              JSON
            </Button>
            <Button
              variant={format === "csv" ? "default" : "outline"}
              size="sm"
              onClick={() => setFormat("csv")}
            >
              <FileSpreadsheet className="h-4 w-4 mr-1" />
              CSV
            </Button>
            <Button
              variant={format === "yaml" ? "default" : "outline"}
              size="sm"
              onClick={() => setFormat("yaml")}
            >
              YAML
            </Button>
          </div>
        </div>

        <div className="flex items-center justify-between pt-4">
          <div className="flex items-center gap-2">
            <Badge variant="secondary">{selectedEntities.length} selected</Badge>
            <span className="text-sm text-muted-foreground">
              {selectedEntities.length === 0 && "Select at least one entity"}
            </span>
          </div>
          <Button
            onClick={handleExport}
            disabled={selectedEntities.length === 0 || isExporting}
          >
            {isExporting ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Exporting...
              </>
            ) : (
              <>
                <Download className="h-4 w-4 mr-2" />
                Export
              </>
            )}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
