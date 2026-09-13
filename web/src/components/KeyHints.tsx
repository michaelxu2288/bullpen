import type { ViewKey } from "./Rail";

const hints: Record<ViewKey, Array<[string, string]>> = {
  board: [
    ["h l", "lane"],
    ["j k", "card"],
    ["space", "advance"],
    ["r", "reload"],
  ],
  sessions: [["r", "reload"]],
  events: [["r", "reload"]],
};

export function KeyHints({ view }: { view: ViewKey }) {
  return (
    <footer className="keys">
      <span>
        <kbd>1-3</kbd>view
      </span>
      {hints[view].map(([key, label]) => (
        <span key={key}>
          <kbd>{key}</kbd>
          {label}
        </span>
      ))}
    </footer>
  );
}
