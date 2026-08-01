import { render, screen } from "@testing-library/react";
import { JsonText } from "./JsonText";

describe("JsonText", () => {
  it("renders hostile values as inert text", () => {
    const payload =
      '<img src=x onerror="window.__pwned=true"><script>alert(1)</script>';
    const { container } = render(<JsonText value={{ payload }} />);
    expect(screen.getByTestId("json-text").textContent).toContain(
      "<img src=x onerror=",
    );
    expect(screen.getByTestId("json-text").textContent).toContain(
      "<script>alert(1)</script>",
    );
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("script")).toBeNull();
    expect(
      (window as unknown as { __pwned?: boolean }).__pwned,
    ).toBeUndefined();
  });
});
