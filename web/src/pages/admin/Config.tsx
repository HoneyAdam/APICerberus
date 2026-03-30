import { useMemo, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { CheckCircle2, RefreshCcw } from "lucide-react";
import { adminApiRequest } from "@/lib/api";
import { YAMLEditor } from "@/components/editor/YAMLEditor";
import { DiffViewer } from "@/components/editor/DiffViewer";
import { ConfirmDialog } from "@/components/shared/ConfirmDialog";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

const SAMPLE_CONFIG = `gateway:
  http_addr: ":8080"
admin:
  addr: ":8081"
  api_key: "secret-admin"
services: []
routes: []
upstreams: []
`;

function validateYAML(value: string) {
  if (!value.trim()) {
    return "Config cannot be empty.";
  }
  if (!value.includes(":")) {
    return "Looks invalid: expected YAML key/value pairs.";
  }
  return "";
}

export function ConfigPage() {
  const [current, setCurrent] = useState(SAMPLE_CONFIG);
  const [edited, setEdited] = useState(SAMPLE_CONFIG);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const error = useMemo(() => validateYAML(edited), [edited]);
  const isDirty = edited !== current;

  const applyMutation = useMutation({
    mutationFn: async () => {
      await adminApiRequest("/admin/api/v1/config/reload", {
        method: "POST",
        body: {},
      });
      return true;
    },
    onSuccess: () => {
      setCurrent(edited);
      setConfirmOpen(false);
    },
  });

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Configuration Editor</CardTitle>
          <CardDescription>Edit YAML, validate and apply runtime reload.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex items-center gap-2">
            {error ? (
              <Badge variant="outline" className="border-destructive/40 text-destructive">
                {error}
              </Badge>
            ) : (
              <Badge variant="outline" className="border-success/40 text-success">
                <CheckCircle2 className="mr-1 size-3.5" />
                YAML looks valid
              </Badge>
            )}
            {isDirty ? <Badge variant="secondary">Unsaved changes</Badge> : null}
          </div>

          <YAMLEditor value={edited} onChange={setEdited} minHeight={360} />

          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={() => setEdited(current)}>
              Reset
            </Button>
            <Button onClick={() => setConfirmOpen(true)} disabled={!isDirty || Boolean(error) || applyMutation.isPending}>
              <RefreshCcw className="mr-2 size-4" />
              Apply Configuration
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Diff View</CardTitle>
          <CardDescription>Side-by-side comparison before apply.</CardDescription>
        </CardHeader>
        <CardContent>
          <DiffViewer leftValue={current} rightValue={edited} leftTitle="Current" rightTitle="Edited" />
        </CardContent>
      </Card>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Apply configuration?"
        description="Gateway will reload runtime configuration."
        confirmLabel="Apply"
        onConfirm={() => applyMutation.mutate()}
      />
    </div>
  );
}

