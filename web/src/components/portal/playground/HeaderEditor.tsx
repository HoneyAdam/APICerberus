import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { newKVRow, type PlaygroundDraft, type PlaygroundKVRow } from "./types";

type HeaderEditorProps = {
  draft: PlaygroundDraft;
  onChange: (updater: (current: PlaygroundDraft) => PlaygroundDraft) => void;
};

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

export function HeaderEditor({ draft, onChange }: HeaderEditorProps) {
  return (
    <div className="space-y-2 rounded-xl border bg-background p-3">
      <div className="flex items-center justify-between gap-2">
        <div>
          <h3 className="text-sm font-semibold">Header Editor</h3>
          <p className="text-xs text-muted-foreground">Dynamic request headers with API key autofill.</p>
        </div>
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={() =>
              onChange((current) => {
                const hasAPIKey = current.headerRows.some((row) => row.key.toLowerCase() === "x-api-key");
                if (hasAPIKey) {
                  return {
                    ...current,
                    headerRows: current.headerRows.map((row) => {
                      if (row.key.toLowerCase() !== "x-api-key") {
                        return row;
                      }
                      return {
                        ...row,
                        value: current.apiKey,
                      };
                    }),
                  };
                }
                return {
                  ...current,
                  headerRows: [...current.headerRows, newKVRow("X-API-Key", current.apiKey)],
                };
              })
            }
          >
            Auto-fill X-API-Key
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() =>
              onChange((current) => ({
                ...current,
                headerRows: [...current.headerRows, newKVRow()],
              }))
            }
          >
            Add Header
          </Button>
        </div>
      </div>

      <div className="space-y-2">
        {draft.headerRows.map((row, index) => (
          <div key={row.id} className="grid gap-2 md:grid-cols-[1fr_1fr_auto]">
            <div className="space-y-1">
              <Label className="text-xs">Key</Label>
              <Input
                value={row.key}
                placeholder="Authorization"
                onChange={(event) =>
                  onChange((current) => ({
                    ...current,
                    headerRows: upsertRows(current.headerRows, index, { key: event.target.value }),
                  }))
                }
              />
            </div>
            <div className="space-y-1">
              <Label className="text-xs">Value</Label>
              <Input
                value={row.value}
                placeholder="Bearer ..."
                onChange={(event) =>
                  onChange((current) => ({
                    ...current,
                    headerRows: upsertRows(current.headerRows, index, { value: event.target.value }),
                  }))
                }
              />
            </div>
            <div className="self-end">
              <Button
                variant="ghost"
                onClick={() =>
                  onChange((current) => ({
                    ...current,
                    headerRows: current.headerRows.filter((item) => item.id !== row.id),
                  }))
                }
              >
                Remove
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
