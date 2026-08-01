import * as Dialog from "@radix-ui/react-dialog";
import { useState } from "react";
import { X } from "lucide-react";
import type { Profile, ProfileInput } from "../api/types";

export function ProfileDialog({
  open,
  onOpenChange,
  profile,
  onSave,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  profile?: Profile;
  onSave: (input: ProfileInput, id?: string) => Promise<void>;
}) {
  const [name, setName] = useState(profile?.name ?? "");
  const [projectId, setProjectId] = useState(profile?.projectId ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setSaving(true);
    setError(null);
    try {
      await onSave({ name, projectId }, profile?.id);
      onOpenChange(false);
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Connection could not be saved",
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content">
          <div className="dialog-head">
            <div>
              <Dialog.Title>
                {profile ? "Edit connection" : "New connection"}
              </Dialog.Title>
              <Dialog.Description>
                Google Cloud profile stored by the local LogLeopard service.
              </Dialog.Description>
            </div>
            <Dialog.Close className="icon-button" aria-label="Close">
              <X size={16} />
            </Dialog.Close>
          </div>
          <form onSubmit={submit} className="dialog-form">
            <label>
              Display name
              <input
                required
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="Production read-only"
              />
            </label>
            <label>
              GCP project ID
              <input
                required
                value={projectId}
                onChange={(event) => setProjectId(event.target.value)}
                placeholder="my-project-123"
              />
            </label>
            <p className="security-note">
              Credentials never enter the browser. Authentication is managed by
              the local service.
            </p>
            {error && (
              <p className="inline-error" role="alert">
                {error}
              </p>
            )}
            <button className="primary-button" disabled={saving}>
              {saving ? "Saving…" : "Save connection"}
            </button>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
