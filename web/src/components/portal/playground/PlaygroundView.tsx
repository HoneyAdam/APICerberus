import { SendHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable";
import type { PlaygroundTemplate, PortalAPIItem, PortalPlaygroundResponse } from "@/lib/portal-types";
import { BodyEditor } from "./BodyEditor";
import { HeaderEditor } from "./HeaderEditor";
import { RequestBuilder } from "./RequestBuilder";
import { ResponseViewer } from "./ResponseViewer";
import { TemplateManager } from "./TemplateManager";
import type { PlaygroundDraft } from "./types";

type PlaygroundViewProps = {
  draft: PlaygroundDraft;
  response: PortalPlaygroundResponse | null;
  sending: boolean;
  apis: PortalAPIItem[];
  templates: PlaygroundTemplate[];
  onDraftChange: (updater: (current: PlaygroundDraft) => PlaygroundDraft) => void;
  onSend: () => Promise<void>;
  onLoadTemplate: (template: PlaygroundTemplate) => void;
  onSaveTemplate: (name: string) => Promise<void>;
  onDeleteTemplate: (templateID: string) => Promise<void>;
};

export function PlaygroundView({
  draft,
  response,
  sending,
  apis,
  templates,
  onDraftChange,
  onSend,
  onLoadTemplate,
  onSaveTemplate,
  onDeleteTemplate,
}: PlaygroundViewProps) {
  return (
    <ResizablePanelGroup orientation="horizontal" className="min-h-[720px] rounded-xl border">
      <ResizablePanel defaultSize={58} minSize={44}>
        <div className="space-y-3 p-3">
          <RequestBuilder draft={draft} apis={apis} onChange={onDraftChange} />
          <HeaderEditor draft={draft} onChange={onDraftChange} />
          <div className="space-y-2 rounded-xl border bg-background p-3">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-semibold">Body Editor</h3>
              <Button size="sm" onClick={onSend} disabled={sending}>
                <SendHorizontal className="mr-2 size-4" />
                {sending ? "Sending..." : "Send"}
              </Button>
            </div>
            <BodyEditor
              value={draft.body}
              onChange={(value) =>
                onDraftChange((current) => ({
                  ...current,
                  body: value,
                }))
              }
            />
          </div>
          <TemplateManager
            templates={templates}
            draft={draft}
            onLoad={onLoadTemplate}
            onSave={onSaveTemplate}
            onDelete={onDeleteTemplate}
          />
        </div>
      </ResizablePanel>

      <ResizableHandle withHandle />

      <ResizablePanel defaultSize={42} minSize={28}>
        <div className="h-full p-3">
          <ResponseViewer response={response} />
        </div>
      </ResizablePanel>
    </ResizablePanelGroup>
  );
}
