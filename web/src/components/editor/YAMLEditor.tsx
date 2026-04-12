import { useEffect, useRef } from "react";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { yaml } from "@codemirror/lang-yaml";
import { oneDark } from "@codemirror/theme-one-dark";
import { useTheme } from "@/components/layout/ThemeProvider";
import { cn } from "@/lib/utils";

type YAMLEditorProps = {
  value: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
  minHeight?: number;
  className?: string;
};

export function YAMLEditor({
  value,
  onChange,
  readOnly = false,
  minHeight = 320,
  className,
}: YAMLEditorProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const editorRef = useRef<EditorView | null>(null);
  const onChangeRef = useRef(onChange);
  const { resolvedMode } = useTheme();

  onChangeRef.current = onChange;

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
      ".cm-activeLineGutter": { backgroundColor: "hsl(var(--accent))" },
      ".cm-activeLine": { backgroundColor: "hsl(var(--accent) / 0.45)" },
      ".cm-selectionBackground, &.cm-focused .cm-selectionBackground": {
        backgroundColor: "hsl(var(--primary) / 0.24)",
      },
    });

    const extensions = [
      yaml(),
      EditorView.lineWrapping,
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          onChangeRef.current?.(update.state.doc.toString());
        }
      }),
      EditorView.editable.of(!readOnly),
      resolvedMode === "dark" ? oneDark : lightTheme,
    ];

    const state = EditorState.create({
      doc: value,
      extensions,
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
  }, [minHeight, readOnly, resolvedMode]);

  useEffect(() => {
    const view = editorRef.current;
    if (!view) {
      return;
    }
    const current = view.state.doc.toString();
    if (current === value) {
      return;
    }
    view.dispatch({
      changes: { from: 0, to: current.length, insert: value },
    });
  }, [value]);

  return (
    <div
      className={cn("overflow-hidden rounded-lg border bg-background", className)}
      style={{ minHeight: `${minHeight}px` }}
      ref={containerRef}
    />
  );
}

