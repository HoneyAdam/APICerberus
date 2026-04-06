import { useState, useCallback } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { adminApiRequest } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Upload, FileJson, FileSpreadsheet, AlertCircle, CheckCircle2, XCircle, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";

export type ImportEntity = "routes" | "services" | "upstreams" | "consumers" | "plugins";

export type ImportResult = {
  success: boolean;
  entity: string;
  id?: string;
  name?: string;
  error?: string;
};

export type ImportPreview = {
  valid: boolean;
  total: number;
  validCount: number;
  invalidCount: number;
  items: Array<{
    index: number;
    valid: boolean;
    data: unknown;
    errors?: string[];
  }>;
};

type BulkImportProps = {
  entity: ImportEntity;
  onSuccess?: () => void;
  className?: string;
};

const ENTITY_ENDPOINTS: Record<ImportEntity, string> = {
  routes: "/admin/api/v1/routes",
  services: "/admin/api/v1/services",
  upstreams: "/admin/api/v1/upstreams",
  consumers: "/admin/api/v1/consumers",
  plugins: "/admin/api/v1/plugins",
};

const ENTITY_QUERY_KEYS: Record<ImportEntity, string[]> = {
  routes: ["routes"],
  services: ["services"],
  upstreams: ["upstreams"],
  consumers: ["consumers"],
  plugins: ["plugins"],
};

export function BulkImport({ entity, onSuccess, className }: BulkImportProps) {
  const [file, setFile] = useState<File | null>(null);
  const [_content, setContent] = useState("");
  const [format, setFormat] = useState<"json" | "csv">("json");
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [results, setResults] = useState<ImportResult[] | null>(null);
  const [showResults, setShowResults] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const queryClient = useQueryClient();

  const validateMutation = useMutation({
    mutationFn: async (data: unknown[]) => {
      const response = await adminApiRequest<ImportPreview>(`/admin/api/v1/${entity}/validate`, {
        method: "POST",
        body: { items: data },
      });
      return response;
    },
  });

  const importMutation = useMutation({
    mutationFn: async (data: unknown[]) => {
      const results: ImportResult[] = [];
      for (const item of data) {
        try {
          const result = await adminApiRequest<Record<string, unknown>>(ENTITY_ENDPOINTS[entity], {
            method: "POST",
            body: item,
          });
          results.push({
            success: true,
            entity,
            id: result.id as string,
            name: result.name as string || (result.id as string),
          });
        } catch (error) {
          results.push({
            success: false,
            entity,
            error: error instanceof Error ? error.message : "Unknown error",
          });
        }
      }
      return results;
    },
    onSuccess: async (data) => {
      setResults(data);
      setShowResults(true);
      await queryClient.invalidateQueries({ queryKey: ENTITY_QUERY_KEYS[entity] });
      onSuccess?.();
    },
  });

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
  }, []);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const droppedFile = e.dataTransfer.files[0];
    if (droppedFile) {
      processFile(droppedFile);
    }
  }, []);

  const handleFileSelect = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const selectedFile = e.target.files?.[0];
    if (selectedFile) {
      processFile(selectedFile);
    }
  }, []);

  const processFile = (f: File) => {
    setFile(f);
    const reader = new FileReader();
    reader.onload = (e) => {
      const text = e.target?.result as string;
      setContent(text);
      parseContent(text, f.name.endsWith(".csv") ? "csv" : "json");
    };
    reader.readAsText(f);
  };

  const parseContent = (text: string, fmt: "json" | "csv") => {
    setFormat(fmt);
    try {
      let data: unknown[];
      if (fmt === "json") {
        data = JSON.parse(text);
        if (!Array.isArray(data)) {
          data = [data];
        }
      } else {
        data = parseCSV(text);
      }
      validateMutation.mutate(data);
    } catch (error) {
      setPreview({
        valid: false,
        total: 0,
        validCount: 0,
        invalidCount: 1,
        items: [{
          index: 0,
          valid: false,
          data: null,
          errors: [error instanceof Error ? error.message : "Parse error"],
        }],
      });
    }
  };

  const parseCSV = (text: string): unknown[] => {
    const lines = text.trim().split("\n");
    if (lines.length < 2) return [];

    const headers = lines[0].split(",").map(h => h.trim().replace(/^"|"$/g, ""));
    const rows: unknown[] = [];

    for (let i = 1; i < lines.length; i++) {
      const values = lines[i].split(",").map(v => v.trim().replace(/^"|"$/g, ""));
      const obj: Record<string, unknown> = {};
      headers.forEach((h, idx) => {
        obj[h] = values[idx] ?? "";
      });
      rows.push(obj);
    }

    return rows;
  };

  const handleImport = () => {
    if (!preview?.items) return;
    const validItems = preview.items.filter(i => i.valid).map(i => i.data);
    importMutation.mutate(validItems);
  };

  const handleClear = () => {
    setFile(null);
    setContent("");
    setPreview(null);
    setResults(null);
    setShowResults(false);
  };

  const successCount = results?.filter(r => r.success).length ?? 0;
  const errorCount = results?.filter(r => !r.success).length ?? 0;

  return (
    <Card className={className}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Upload className="h-5 w-5" />
          Bulk Import {entity.charAt(0).toUpperCase() + entity.slice(1)}
        </CardTitle>
        <CardDescription>
          Import multiple {entity} from JSON or CSV file
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-4">
        <Tabs value={format} onValueChange={(v) => setFormat(v as "json" | "csv")}>
          <TabsList>
            <TabsTrigger value="json">
              <FileJson className="h-4 w-4 mr-1" />
              JSON
            </TabsTrigger>
            <TabsTrigger value="csv">
              <FileSpreadsheet className="h-4 w-4 mr-1" />
              CSV
            </TabsTrigger>
          </TabsList>

          <TabsContent value="json" className="mt-4">
            <div
              onDragOver={handleDragOver}
              onDragLeave={handleDragLeave}
              onDrop={handleDrop}
              className={cn(
                "border-2 border-dashed rounded-lg p-8 text-center transition-colors",
                dragOver ? "border-primary bg-primary/5" : "border-muted-foreground/25",
                file && "border-success bg-success/5"
              )}
            >
              <Upload className="h-8 w-8 mx-auto mb-2 text-muted-foreground" />
              <p className="text-sm text-muted-foreground mb-2">
                Drag and drop a JSON file, or click to browse
              </p>
              <input
                type="file"
                accept=".json"
                onChange={handleFileSelect}
                className="hidden"
                id={`json-upload-${entity}`}
              />
              <Button asChild variant="outline" size="sm">
                <label htmlFor={`json-upload-${entity}`}>Select File</label>
              </Button>
              {file && (
                <p className="mt-2 text-sm">
                  <Badge variant="outline">{file.name}</Badge>
                </p>
              )}
            </div>
          </TabsContent>

          <TabsContent value="csv" className="mt-4">
            <div
              onDragOver={handleDragOver}
              onDragLeave={handleDragLeave}
              onDrop={handleDrop}
              className={cn(
                "border-2 border-dashed rounded-lg p-8 text-center transition-colors",
                dragOver ? "border-primary bg-primary/5" : "border-muted-foreground/25",
                file && "border-success bg-success/5"
              )}
            >
              <Upload className="h-8 w-8 mx-auto mb-2 text-muted-foreground" />
              <p className="text-sm text-muted-foreground mb-2">
                Drag and drop a CSV file, or click to browse
              </p>
              <input
                type="file"
                accept=".csv"
                onChange={handleFileSelect}
                className="hidden"
                id={`csv-upload-${entity}`}
              />
              <Button asChild variant="outline" size="sm">
                <label htmlFor={`csv-upload-${entity}`}>Select File</label>
              </Button>
              {file && (
                <p className="mt-2 text-sm">
                  <Badge variant="outline">{file.name}</Badge>
                </p>
              )}
            </div>
          </TabsContent>
        </Tabs>

        {validateMutation.isPending && (
          <Alert>
            <Loader2 className="h-4 w-4 animate-spin" />
            <AlertTitle>Validating</AlertTitle>
            <AlertDescription>Checking file format and data...</AlertDescription>
          </Alert>
        )}

        {preview && (
          <div className="space-y-4">
            <div className="flex items-center gap-2">
              <Badge variant={preview.valid ? "default" : "destructive"}>
                {preview.validCount} valid
              </Badge>
              {preview.invalidCount > 0 && (
                <Badge variant="destructive">{preview.invalidCount} invalid</Badge>
              )}
              <span className="text-sm text-muted-foreground">
                of {preview.total} total items
              </span>
            </div>

            {preview.invalidCount > 0 && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertTitle>Validation Errors</AlertTitle>
                <AlertDescription>
                  Found {preview.invalidCount} invalid items. Please fix before importing.
                </AlertDescription>
              </Alert>
            )}

            <ScrollArea className="h-64 rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-16">#</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Data Preview</TableHead>
                    <TableHead>Errors</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {preview.items.map((item) => (
                    <TableRow key={item.index}>
                      <TableCell>{item.index + 1}</TableCell>
                      <TableCell>
                        {item.valid ? (
                          <CheckCircle2 className="h-4 w-4 text-success" />
                        ) : (
                          <XCircle className="h-4 w-4 text-destructive" />
                        )}
                      </TableCell>
                      <TableCell>
                        <code className="text-xs bg-muted px-1 py-0.5 rounded">
                          {JSON.stringify(item.data).slice(0, 50)}...
                        </code>
                      </TableCell>
                      <TableCell className="text-xs text-destructive">
                        {item.errors?.join(", ")}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </ScrollArea>

            <div className="flex gap-2">
              <Button
                onClick={handleImport}
                disabled={!preview.valid || importMutation.isPending}
              >
                {importMutation.isPending ? (
                  <>
                    <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                    Importing...
                  </>
                ) : (
                  <>
                    <Upload className="h-4 w-4 mr-2" />
                    Import {preview.validCount} Items
                  </>
                )}
              </Button>
              <Button variant="outline" onClick={handleClear}>
                Clear
              </Button>
            </div>
          </div>
        )}

        <Dialog open={showResults} onOpenChange={setShowResults}>
          <DialogContent className="max-w-2xl">
            <DialogHeader>
              <DialogTitle>Import Results</DialogTitle>
              <DialogDescription>
                Successfully imported {successCount} of {results?.length} items
              </DialogDescription>
            </DialogHeader>

            <div className="flex gap-2 mb-4">
              <Badge variant="default">{successCount} success</Badge>
              {errorCount > 0 && <Badge variant="destructive">{errorCount} failed</Badge>}
            </div>

            <ScrollArea className="h-64 rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Status</TableHead>
                    <TableHead>ID/Name</TableHead>
                    <TableHead>Error</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {results?.map((result, idx) => (
                    <TableRow key={idx}>
                      <TableCell>
                        {result.success ? (
                          <CheckCircle2 className="h-4 w-4 text-success" />
                        ) : (
                          <XCircle className="h-4 w-4 text-destructive" />
                        )}
                      </TableCell>
                      <TableCell>{result.name || result.id || "-"}</TableCell>
                      <TableCell className="text-xs text-destructive">{result.error}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </ScrollArea>

            <DialogFooter>
              <Button onClick={() => { setShowResults(false); handleClear(); }}>
                Done
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardContent>
    </Card>
  );
}
