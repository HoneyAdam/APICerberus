import { useMemo, useState } from "react";
import { toast } from "sonner";
import { PlaygroundView } from "@/components/portal/playground/PlaygroundView";
import {
  newKVRow,
  recordToRows,
  rowsToRecord,
  type PlaygroundDraft,
} from "@/components/portal/playground/types";
import {
  useDeletePortalTemplate,
  usePortalAPIs,
  usePortalPlaygroundSend,
  usePortalTemplates,
  useSavePortalTemplate,
} from "@/hooks/use-portal";
import type { PlaygroundTemplate, PortalPlaygroundResponse } from "@/lib/portal-types";

const DEFAULT_DRAFT: PlaygroundDraft = {
  method: "GET",
  path: "/",
  apiKey: "",
  timeoutMs: 30_000,
  body: "{\n  \"hello\": \"world\"\n}",
  queryRows: [newKVRow()],
  headerRows: [newKVRow("Content-Type", "application/json")],
};

export function PortalPlaygroundPage() {
  const [draft, setDraft] = useState<PlaygroundDraft>(DEFAULT_DRAFT);
  const [response, setResponse] = useState<PortalPlaygroundResponse | null>(null);

  const apisQuery = usePortalAPIs();
  const templatesQuery = usePortalTemplates();
  const sendMutation = usePortalPlaygroundSend();
  const saveTemplateMutation = useSavePortalTemplate();
  const deleteTemplateMutation = useDeletePortalTemplate();

  const apiItems = useMemo(() => apisQuery.data?.items ?? [], [apisQuery.data?.items]);
  const templates = useMemo(() => templatesQuery.data?.items ?? [], [templatesQuery.data?.items]);

  const onSend = async () => {
    try {
      const payload = {
        method: draft.method,
        path: draft.path,
        query: rowsToRecord(draft.queryRows),
        headers: rowsToRecord(draft.headerRows),
        body: draft.body,
        api_key: draft.apiKey.trim(),
        timeout_ms: draft.timeoutMs,
      };
      const result = await sendMutation.mutateAsync(payload);
      setResponse(result);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Playground request failed");
    }
  };

  const onLoadTemplate = (template: PlaygroundTemplate) => {
    setDraft((current) => ({
      ...current,
      method: template.method || current.method,
      path: template.path || current.path,
      body: template.body || current.body,
      headerRows: recordToRows(template.headers),
      queryRows: recordToRows(template.query),
    }));
    toast.success(`Loaded template: ${template.name}`);
  };

  const onSaveTemplate = async (name: string) => {
    try {
      await saveTemplateMutation.mutateAsync({
        name,
        method: draft.method,
        path: draft.path,
        query: rowsToRecord(draft.queryRows),
        headers: rowsToRecord(draft.headerRows),
        body: draft.body,
      });
      toast.success("Template saved");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save template");
    }
  };

  const onDeleteTemplate = async (templateID: string) => {
    try {
      await deleteTemplateMutation.mutateAsync(templateID);
      toast.success("Template deleted");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to delete template");
    }
  };

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-2xl font-semibold">Playground</h2>
        <p className="text-sm text-muted-foreground">Compose and run live requests against your allowed APIs.</p>
      </div>

      <PlaygroundView
        draft={draft}
        response={response}
        sending={sendMutation.isPending}
        apis={apiItems}
        templates={templates}
        onDraftChange={(updater) => setDraft((current) => updater(current))}
        onSend={onSend}
        onLoadTemplate={onLoadTemplate}
        onSaveTemplate={onSaveTemplate}
        onDeleteTemplate={onDeleteTemplate}
      />
    </div>
  );
}
