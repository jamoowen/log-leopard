import { Plus, Trash2 } from "lucide-react";
import {
  validatePredicate,
  type PredicateDraft,
  type PredicateOperator,
} from "../query-tools";

const operators: PredicateOperator[] = [
  "equals",
  "contains",
  "exists",
  "gt",
  "lt",
];

export function StructuredBuilder({
  drafts,
  onChange,
}: {
  drafts: PredicateDraft[];
  onChange: (drafts: PredicateDraft[]) => void;
}) {
  function patch(id: string, value: Partial<PredicateDraft>) {
    onChange(
      drafts.map((draft) => (draft.id === id ? { ...draft, ...value } : draft)),
    );
  }
  function add() {
    if (drafts.length < 50)
      onChange([
        ...drafts,
        { id: crypto.randomUUID(), path: "", operator: "equals", value: "" },
      ]);
  }
  return (
    <div className="predicate-builder">
      <div className="predicate-head">
        <strong>All conditions must match</strong>
        <span>{drafts.length}/50</span>
      </div>
      {drafts.length === 0 && (
        <button className="empty-builder" onClick={add}>
          <Plus size={14} /> Add the first structured condition
        </button>
      )}
      {drafts.map((draft, index) => {
        const error = validatePredicate(draft);
        return (
          <div className="predicate-row" key={draft.id}>
            <span className="predicate-index">{index + 1}</span>
            <label>
              <span>JSON path</span>
              <input
                aria-label={`Condition ${index + 1} path`}
                value={draft.path}
                onChange={(event) =>
                  patch(draft.id, { path: event.target.value })
                }
                placeholder="request.latencyMs"
              />
            </label>
            <label>
              <span>Operator</span>
              <select
                aria-label={`Condition ${index + 1} operator`}
                value={draft.operator}
                onChange={(event) =>
                  patch(draft.id, {
                    operator: event.target.value as PredicateOperator,
                    value:
                      event.target.value === "exists" ? "true" : draft.value,
                  })
                }
              >
                {operators.map((operator) => (
                  <option value={operator} key={operator}>
                    {operator}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <span>Typed value</span>
              {draft.operator === "exists" ? (
                <select
                  aria-label={`Condition ${index + 1} value`}
                  value={draft.value}
                  onChange={(event) =>
                    patch(draft.id, { value: event.target.value })
                  }
                >
                  <option value="true">true</option>
                  <option value="false">false</option>
                </select>
              ) : (
                <input
                  aria-label={`Condition ${index + 1} value`}
                  inputMode={
                    draft.operator === "gt" || draft.operator === "lt"
                      ? "decimal"
                      : undefined
                  }
                  value={draft.value}
                  onChange={(event) =>
                    patch(draft.id, { value: event.target.value })
                  }
                  placeholder={
                    draft.operator === "contains"
                      ? "text"
                      : "text, number, or boolean"
                  }
                />
              )}
            </label>
            <button
              className="icon-button"
              aria-label={`Remove condition ${index + 1}`}
              onClick={() =>
                onChange(drafts.filter((item) => item.id !== draft.id))
              }
            >
              <Trash2 size={13} />
            </button>
            {error && <small className="predicate-error">{error}</small>}
          </div>
        );
      })}
      {drafts.length > 0 && (
        <button
          className="add-condition"
          disabled={drafts.length >= 50}
          onClick={add}
        >
          <Plus size={13} /> Add condition
        </button>
      )}
    </div>
  );
}
