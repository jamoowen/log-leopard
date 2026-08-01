import { useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { Search } from "lucide-react";

export interface Command {
  id: string;
  label: string;
  group: string;
  hint?: string;
  run: () => void;
}

export function CommandPalette({
  open,
  onOpenChange,
  commands,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  commands: Command[];
}) {
  const [filter, setFilter] = useState("");
  function changeOpen(next: boolean) {
    if (!next) setFilter("");
    onOpenChange(next);
  }
  const visible = commands.filter((command) =>
    `${command.label} ${command.group}`
      .toLowerCase()
      .includes(filter.toLowerCase()),
  );
  return (
    <Dialog.Root open={open} onOpenChange={changeOpen}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content
          className="command-content"
          aria-describedby={undefined}
        >
          <Dialog.Title className="sr-only">Command palette</Dialog.Title>
          <div className="command-search">
            <Search size={15} />
            <input
              aria-label="Search commands"
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
              placeholder="Type a command…"
            />
          </div>
          <div className="command-list">
            {visible.map((command) => (
              <button
                key={command.id}
                onClick={() => {
                  command.run();
                  onOpenChange(false);
                }}
              >
                <span>
                  <small>{command.group}</small>
                  {command.label}
                </span>
                {command.hint && <kbd>{command.hint}</kbd>}
              </button>
            ))}
            {visible.length === 0 && <p>No matching command.</p>}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
