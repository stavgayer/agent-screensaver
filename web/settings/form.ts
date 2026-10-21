/** One option as an extension declares it in `extension.json` (see docs/extensions.md). */
export interface OptionSpec {
  type: "string" | "number" | "boolean" | "color" | "enum" | "file";
  default: unknown;
  min?: number;
  max?: number;
  values?: string[];
  label?: string;
}

export type Control = "text" | "number" | "toggle" | "color" | "select" | "file";

/** One form field, ready to render. */
export type Field = {
  key: string;
  label: string;
  control: Control;
  value: unknown;
  min?: number;
  max?: number;
  choices?: string[];
};

const CONTROLS: Record<OptionSpec["type"], Control> = {
  string: "text",
  number: "number",
  boolean: "toggle",
  color: "color",
  enum: "select",
  file: "file",
};

/**
 * Turns an extension's option schema into form fields, in the order the
 * options are listed. The user's value wins over the default. This is the
 * one place that maps an option kind to a control, so a new extension gets
 * its form with no settings code.
 */
export function formFor(options: Record<string, OptionSpec>, values: Record<string, unknown>): Field[] {
  return Object.entries(options).map(([key, spec]) => {
    const field: Field = {
      key,
      label: spec.label || key,
      control: CONTROLS[spec.type] ?? "text",
      value: key in values ? values[key] : spec.default,
    };
    if (spec.type === "number") {
      if (spec.min !== undefined) field.min = spec.min;
      if (spec.max !== undefined) field.max = spec.max;
    }
    if (spec.type === "enum") field.choices = spec.values ?? [];
    return field;
  });
}
