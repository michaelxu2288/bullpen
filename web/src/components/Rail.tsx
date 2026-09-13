export type ViewKey = "board" | "sessions" | "events";

interface Props {
  active: ViewKey;
  counts: Partial<Record<ViewKey, number>>;
  onSelect: (view: ViewKey) => void;
}

const items: Array<{ key: ViewKey; label: string; hotkey: string }> = [
  { key: "board", label: "board", hotkey: "1" },
  { key: "sessions", label: "fleet", hotkey: "2" },
  { key: "events", label: "events", hotkey: "3" },
];

export function Rail({ active, counts, onSelect }: Props) {
  return (
    <nav className="rail" aria-label="views">
      {items.map((item) => (
        <button
          key={item.key}
          type="button"
          className="rail__item"
          aria-current={active === item.key}
          onClick={() => onSelect(item.key)}
        >
          <span className="rail__key">{item.hotkey}</span>
          <span>{item.label}</span>
          <span className="rail__count">{counts[item.key] ?? ""}</span>
        </button>
      ))}
      <p className="rail__note">
        bullpen
        <br />
        master + workers
      </p>
    </nav>
  );
}
