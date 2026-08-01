import {
  toPredicate,
  validatePredicate,
  type PredicateDraft,
} from "./query-tools";

const draft = (patch: Partial<PredicateDraft>): PredicateDraft => ({
  id: "1",
  path: "request.latencyMs",
  operator: "equals",
  value: "value",
  ...patch,
});

describe("structured predicates", () => {
  it("converts values according to the generated operator contract", () => {
    expect(toPredicate(draft({ value: "42" }))?.value).toBe(42);
    expect(toPredicate(draft({ value: "true" }))?.value).toBe(true);
    expect(
      toPredicate(draft({ operator: "contains", value: "42" }))?.value,
    ).toBe("42");
    expect(toPredicate(draft({ operator: "gt", value: "4.2" }))?.value).toBe(
      4.2,
    );
    expect(
      toPredicate(draft({ operator: "exists", value: "false" }))?.value,
    ).toBe(false);
  });
  it("rejects invalid paths and operator values inline", () => {
    expect(validatePredicate(draft({ path: "bad-key.x" }))).toBeTruthy();
    expect(
      validatePredicate(draft({ path: "__proto__.polluted" })),
    ).toBeTruthy();
    expect(
      validatePredicate(draft({ operator: "lt", value: "nope" })),
    ).toBeTruthy();
  });
});
