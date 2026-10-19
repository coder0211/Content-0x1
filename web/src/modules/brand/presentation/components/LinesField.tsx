import { forwardRef, useEffect, useState, type TextareaHTMLAttributes } from "react";
import { Textarea } from "@/shared/ui/textarea";

type Props = Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, "value" | "onChange"> & {
  value: string[];
  onChange: (lines: string[]) => void;
};

const toText = (lines: string[]) => lines.join("\n");

/** Edits a string list as "one item per line" (work-tool style, keyboard friendly). */
export const LinesField = forwardRef<HTMLTextAreaElement, Props>(({ value, onChange, rows = 3, ...props }, ref) => {
  const [text, setText] = useState(() => toText(value));
  useEffect(() => {
    // Sync when the parent resets the value (e.g. after reload), not on our own edits.
    setText((cur) => (toText(parse(cur)) === toText(value) ? cur : toText(value)));
  }, [value]);
  return (
    <Textarea
      ref={ref}
      rows={rows}
      value={text}
      onChange={(e) => {
        setText(e.target.value);
        onChange(parse(e.target.value));
      }}
      {...props}
    />
  );
});
LinesField.displayName = "LinesField";

function parse(text: string): string[] {
  return text
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
}
