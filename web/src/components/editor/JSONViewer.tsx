import { useEffect, useMemo, useRef } from "react";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { json } from "@codemirror/lang-json";
import { oneDark } from "@codemirror/theme-one-dark";
import { useTheme } from "@/components/layout/ThemeProvider";
import { cn } from "@/lib/utils";

type JSONViewerProps = {
  value: unknown;
  minHeight?: number;
  className?: string;
};

function normalizeJSON(value: unknown) {
  if (typeof value === "string") {
    return value;
  }
  return JSON.stringify(value, null, 2);
}

export function JSONViewer({ value, minHeight = 280, className }: JSONViewerProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const editorRef = useRef<EditorView | null>(null);
  const { resolvedMode } = useTheme();

  const textValue = useMemo(() => normalizeJSON(value), [value]);

  useEffect(() => {
    if (!containerRef.current) {
      return;
    }

    const lightTheme = EditorView.theme({
      "&": {
        height: "100%",
        fontFamily: "\"JetBrains Mono Variable\", ui-monospace, SFMono-Regular, Menlo, monospace",
      },
      ".cm-scroller": { overflow: "auto" },
      ".cm-content": { minHeight: `${minHeight}px` },
      ".cm-gutters": {
        borderRight: "1px solid hsl(var(--border))",
        backgroundColor: "hsl(var(--muted) / 0.3)",
      },
    });

    const state = EditorState.create({
      doc: textValue,
      extensions: [json(), EditorView.lineWrapping, EditorView.editable.of(false), resolvedMode === "dark" ? oneDark : lightTheme],
    });

    const view = new EditorView({
      state,
      parent: containerRef.current,
    });

    editorRef.current = view;
    return () => {
      view.destroy();
      editorRef.current = null;
    };
  }, [minHeight, resolvedMode, textValue]);

  return (
    <div
      ref={containerRef}
      className={cn("overflow-hidden rounded-lg border bg-background", className)}
      style={{ minHeight: `${minHeight}px` }}
    />
  );
}

