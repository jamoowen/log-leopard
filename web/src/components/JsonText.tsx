import { useState } from "react";
import { Check, Copy } from "lucide-react";

export function JsonText({
  value,
  label = "JSON",
}: {
  value: unknown;
  label?: string;
}) {
  const [copied, setCopied] = useState(false);
  const text = JSON.stringify(value, null, 2);

  async function copy() {
    await navigator.clipboard.writeText(text);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1200);
  }

  return (
    <div className="json-wrap">
      <button
        className="copy-button"
        onClick={copy}
        aria-label={`Copy ${label}`}
      >
        {copied ? <Check size={13} /> : <Copy size={13} />}{" "}
        {copied ? "Copied" : "Copy"}
      </button>
      <pre className="json-text" data-testid="json-text">
        <SyntaxText text={text} />
      </pre>
    </div>
  );
}

function SyntaxText({ text }: { text: string }) {
  const tokens = text.split(
    /("(?:\\.|[^"\\])*")(?=\s*:)|("(?:\\.|[^"\\])*")|(-?\d+(?:\.\d+)?(?:e[+-]?\d+)?|\btrue\b|\bfalse\b|\bnull\b)/gi,
  );
  return tokens.map((token, index) => {
    if (!token) return null;
    const next = tokens.slice(index + 1).find(Boolean);
    let kind = "";
    if (token.startsWith('"'))
      kind = next?.trimStart().startsWith(":") ? "json-key" : "json-string";
    else if (/^-?\d/.test(token)) kind = "json-number";
    else if (/^(true|false|null)$/.test(token)) kind = "json-literal";
    return (
      <span className={kind} key={index}>
        {token}
      </span>
    );
  });
}
