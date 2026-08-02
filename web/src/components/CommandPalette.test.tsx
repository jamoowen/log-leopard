import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { CommandPalette, type Command } from "./CommandPalette";

function renderPalette(commands: Command[]) {
  function Harness() {
    const [open, setOpen] = useState(true);
    return (
      <>
        <button type="button" onClick={() => setOpen(true)}>
          Open palette
        </button>
        <CommandPalette
          open={open}
          onOpenChange={setOpen}
          commands={commands}
        />
      </>
    );
  }
  return render(<Harness />);
}

test("runs the active command with arrow keys and Enter", async () => {
  const first = vi.fn<() => void>();
  const second = vi.fn<() => void>();
  const user = userEvent.setup();
  renderPalette([
    { id: "first", label: "First", group: "Test", run: first },
    { id: "second", label: "Second", group: "Test", run: second },
  ]);
  const search = screen.getByRole("combobox", { name: "Search commands" });

  expect(screen.getByRole("option", { name: /First/ })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await user.type(search, "{ArrowDown}{Enter}");

  expect(first).not.toHaveBeenCalled();
  expect(second).toHaveBeenCalledOnce();
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("resets filtering after a command closes the palette", async () => {
  const user = userEvent.setup();
  renderPalette([
    {
      id: "fleet",
      label: "Open Fleet",
      group: "Navigation",
      run: vi.fn<() => void>(),
    },
    {
      id: "logs",
      label: "Open Logs",
      group: "Navigation",
      run: vi.fn<() => void>(),
    },
  ]);
  const search = screen.getByRole("combobox", { name: "Search commands" });
  await user.type(search, "logs{Enter}");

  await user.click(screen.getByRole("button", { name: "Open palette" }));

  expect(screen.getByRole("combobox", { name: "Search commands" })).toHaveValue(
    "",
  );
  expect(screen.getAllByRole("option")).toHaveLength(2);
});
