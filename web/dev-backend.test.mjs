import { describe, expect, test } from "vitest";
import { validateDevBackend } from "./dev-backend.mjs";

describe("development backend target", () => {
  test("accepts only credential-free numeric loopback origins", () => {
    expect(validateDevBackend("http://127.0.0.1:8787")).toBe(
      "http://127.0.0.1:8787",
    );
    expect(validateDevBackend("http://[::1]:8787")).toBe("http://[::1]:8787");
    for (const target of [
      "https://127.0.0.1:8787",
      "http://localhost:8787",
      "http://example.com",
      "http://user:password@127.0.0.1:8787",
      "http://127.0.0.1:8787/path",
    ]) {
      expect(() => validateDevBackend(target)).toThrow(/numeric loopback/);
    }
  });
});
