import { useEffect, useRef } from "react";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { json } from "@codemirror/lang-json";
import { oneDark } from "@codemirror/theme-one-dark";
import { useTheme } from "@/components/layout/ThemeProvider";
import { cn } from "@/lib/utils";

type BodyEditorProps = {
  value: string;
  onChange: (value: string) => void;
};

export function BodyEditor({ value, onChange }: BodyEditorProps) {
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
        fontFamily: '"Geist Mono Variable", ui-monospace, SFMono-Regular, Menlo, monospace',
      },
      ".cm-scroller": { overflow: "auto" },
      ".cm-content": { minHeight: "180px" },
      ".cm-gutters": {
        borderRight: "1px solid hsl(var(--border))",
        backgroundColor: "hsl(var(--muted) / 0.3)",
      },
    });

    const state = EditorState.create({
      doc: value,
      extensions: [
        json(),
        EditorView.lineWrapping,
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            onChangeRef.current(update.state.doc.toString());
          }
        }),
        resolvedMode === "dark" ? oneDark : lightTheme,
      ],
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
  }, [resolvedMode, value]);

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

  return <div ref={containerRef} className={cn("overflow-hidden rounded-xl border bg-background")} />;
}
