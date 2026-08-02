import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";
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
  returnFocusRef,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  commands: Command[];
  returnFocusRef?: RefObject<HTMLElement | null>;
}) {
  const [filter, setFilter] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const optionRefs = useRef(new Map<string, HTMLButtonElement>());
  function changeOpen(next: boolean) {
    if (!next) setFilter("");
    setActiveIndex(0);
    onOpenChange(next);
  }
  const visible = commands.filter((command) =>
    `${command.label} ${command.group}`
      .toLowerCase()
      .includes(filter.toLowerCase()),
  );
  const boundedIndex = Math.min(activeIndex, Math.max(visible.length - 1, 0));
  const activeCommand = visible[boundedIndex];
  useEffect(() => {
    if (open && activeCommand)
      optionRefs.current
        .get(activeCommand.id)
        ?.scrollIntoView?.({ block: "nearest" });
  }, [activeCommand, open]);

  function run(command: Command) {
    command.run();
    changeOpen(false);
  }

  function navigate(event: KeyboardEvent<HTMLInputElement>) {
    if (!visible.length) return;
    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        setActiveIndex((boundedIndex + 1) % visible.length);
        break;
      case "ArrowUp":
        event.preventDefault();
        setActiveIndex((boundedIndex - 1 + visible.length) % visible.length);
        break;
      case "Home":
        event.preventDefault();
        setActiveIndex(0);
        break;
      case "End":
        event.preventDefault();
        setActiveIndex(visible.length - 1);
        break;
      case "Enter":
        event.preventDefault();
        if (activeCommand) run(activeCommand);
        break;
    }
  }
  return (
    <Dialog.Root open={open} onOpenChange={changeOpen}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content
          className="command-content"
          aria-describedby={undefined}
          onCloseAutoFocus={(event) => {
            if (!returnFocusRef?.current) return;
            event.preventDefault();
            returnFocusRef.current.focus();
          }}
        >
          <Dialog.Title className="sr-only">Command palette</Dialog.Title>
          <div className="command-search">
            <Search size={15} />
            <input
              aria-label="Search commands"
              role="combobox"
              aria-autocomplete="list"
              aria-controls="command-list"
              aria-expanded={open}
              aria-activedescendant={
                activeCommand ? `command-option-${activeCommand.id}` : undefined
              }
              value={filter}
              onChange={(event) => {
                setFilter(event.target.value);
                setActiveIndex(0);
              }}
              onKeyDown={navigate}
              placeholder="Type a command…"
            />
          </div>
          <div className="command-list" id="command-list" role="listbox">
            {visible.map((command, index) => (
              <button
                key={command.id}
                id={`command-option-${command.id}`}
                ref={(element) => {
                  if (element) optionRefs.current.set(command.id, element);
                  else optionRefs.current.delete(command.id);
                }}
                role="option"
                aria-selected={index === boundedIndex}
                className={index === boundedIndex ? "active" : ""}
                onMouseEnter={() => setActiveIndex(index)}
                onFocus={() => setActiveIndex(index)}
                onClick={() => run(command)}
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
