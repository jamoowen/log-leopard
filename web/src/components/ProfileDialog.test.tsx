import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { ProfileDialog } from "./ProfileDialog";

test("keeps the connection dialog open and displays save failures", async () => {
  const user = userEvent.setup();
  const onOpenChange = vi.fn<(open: boolean) => void>();
  const onSave = vi
    .fn<() => Promise<void>>()
    .mockRejectedValue(new Error("Profile storage is unavailable"));

  render(<ProfileDialog open onOpenChange={onOpenChange} onSave={onSave} />);
  await user.type(screen.getByLabelText("Display name"), "Staging");
  await user.type(screen.getByLabelText("GCP project ID"), "example-stg");
  await user.click(screen.getByRole("button", { name: "Save connection" }));

  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Profile storage is unavailable",
  );
  expect(onOpenChange).not.toHaveBeenCalledWith(false);
});
