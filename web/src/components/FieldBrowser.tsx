import { Pin, PinOff, Plus } from "lucide-react";
import type { DiscoveredField } from "../field-browser";

export function FieldBrowser({
  fields,
  pins,
  onTogglePin,
  onAdd,
}: {
  fields: DiscoveredField[];
  pins: string[];
  onTogglePin: (path: string) => void;
  onAdd: (path: string, type: string) => void;
}) {
  return (
    <section className="field-browser">
      <div className="field-browser-head">
        <div>
          <strong>Loaded fields</strong>
          <span>Structured payloads only</span>
        </div>
        <span>{fields.length} paths</span>
      </div>
      {fields.length === 0 ? (
        <p>No structured fields in loaded results.</p>
      ) : (
        <div className="field-list">
          {fields.map((field) => (
            <div className="field-item" key={field.path}>
              <code>{field.path}</code>
              <span>
                {field.types.join(" | ")} · {field.count}
              </span>
              {field.sample && (
                <small title={field.sample}>{field.sample}</small>
              )}
              <button
                aria-label={`${pins.includes(field.path) ? "Unpin" : "Pin"} ${field.path}`}
                title={pins.includes(field.path) ? "Unpin field" : "Pin field"}
                onClick={() => onTogglePin(field.path)}
              >
                {pins.includes(field.path) ? (
                  <PinOff size={12} />
                ) : (
                  <Pin size={12} />
                )}
              </button>
              <button
                aria-label={`Add predicate for ${field.path}`}
                title="Add structured predicate"
                onClick={() => onAdd(field.path, field.types[0] ?? "unknown")}
              >
                <Plus size={12} />
              </button>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
