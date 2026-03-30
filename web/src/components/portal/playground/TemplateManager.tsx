import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
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
import type { PlaygroundTemplate } from "@/lib/portal-types";
import type { PlaygroundDraft } from "./types";

type TemplateManagerProps = {
  templates: PlaygroundTemplate[];
  draft: PlaygroundDraft;
  onLoad: (template: PlaygroundTemplate) => void;
  onSave: (name: string) => Promise<void>;
  onDelete: (templateID: string) => Promise<void>;
};

export function TemplateManager({ templates, draft, onLoad, onSave, onDelete }: TemplateManagerProps) {
  const [open, setOpen] = useState(false);
  const [templateName, setTemplateName] = useState("");

  const suggestedName = useMemo(() => {
    const normalizedPath = draft.path.trim().replaceAll("/", "-").replaceAll("--", "-");
    if (normalizedPath) {
      return `${draft.method.toLowerCase()}${normalizedPath}`;
    }
    return `${draft.method.toLowerCase()}-template`;
  }, [draft.method, draft.path]);

  return (
    <div className="rounded-xl border bg-background p-3">
      <div className="mb-3 flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold">Template Manager</h3>
          <p className="text-xs text-muted-foreground">Save and reuse request templates.</p>
        </div>

        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button size="sm">Save Template</Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Save Template</DialogTitle>
              <DialogDescription>Store the current request builder state as a reusable template.</DialogDescription>
            </DialogHeader>
            <div className="space-y-2">
              <Label htmlFor="template-name">Template Name</Label>
              <Input
                id="template-name"
                value={templateName}
                onChange={(event) => setTemplateName(event.target.value)}
                placeholder={suggestedName}
              />
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button
                onClick={async () => {
                  const name = templateName.trim() || suggestedName;
                  await onSave(name);
                  setTemplateName("");
                  setOpen(false);
                }}
              >
                Save
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      <div className="space-y-2">
        {templates.map((template) => (
          <div key={template.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border p-2">
            <div>
              <p className="text-sm font-medium">{template.name}</p>
              <p className="text-xs text-muted-foreground">
                {template.method} {template.path}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Button size="sm" variant="outline" onClick={() => onLoad(template)}>
                Load
              </Button>
              <Button size="sm" variant="ghost" onClick={() => onDelete(template.id)}>
                Delete
              </Button>
            </div>
          </div>
        ))}
        {templates.length === 0 ? <p className="text-xs text-muted-foreground">No templates saved yet.</p> : null}
      </div>
    </div>
  );
}
