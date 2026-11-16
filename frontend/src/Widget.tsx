import { useEffect, useState } from "react";
import { fmtSpeed } from "./format";
import { Icon } from "./icons";

// The floating drop target. It runs in its own small always-on-top window
// (ForgeDM.exe --widget) and talks to the main app through window.go.app.Widget.
interface Status {
  speed: number;
  active: number;
  queued: number;
}

const W = () => window.go?.app?.Widget;

export function Widget() {
  const [st, setSt] = useState<Status | null>(null);
  const [over, setOver] = useState(false);
  const [flash, setFlash] = useState("");
  const [offline, setOffline] = useState(false);

  useEffect(() => {
    document.body.classList.add("widget-body");
    const t = setInterval(async () => {
      try {
        const s = await W()?.Status();
        setSt(s ?? null);
        setOffline(!s);
      } catch {
        setOffline(true);
      }
    }, 1000);
    return () => clearInterval(t);
  }, []);

  const drop = async (e: React.DragEvent) => {
    e.preventDefault();
    setOver(false);
    const text = e.dataTransfer.getData("text/uri-list") || e.dataTransfer.getData("text/plain");
    if (!text) return;
    try {
      const n: number = await W()?.Drop(text);
      setFlash(n > 0 ? `Added ${n}` : "No link found");
    } catch {
      setFlash("ForgeDM isn't running");
    }
    setTimeout(() => setFlash(""), 2000);
  };

  return (
    <div
      className={`widget${over ? " over" : ""}`}
      onDragOver={(e) => { e.preventDefault(); setOver(true); }}
      onDragLeave={() => setOver(false)}
      onDrop={drop}
      onDoubleClick={() => W()?.OpenMain()}
      title="Drag links here to download. Double-click to open ForgeDM."
    >
      <span style={{ width: 36, height: 36, borderRadius: "50%", background: "#2EE6D6", display: "grid", placeItems: "center", flex: "none", color: "#03201D" }}>
        <Icon name="down" size={17} stroke={2.4} />
      </span>
      {flash ? (
        <span style={{ fontWeight: 600, fontSize: 13 }}>{flash}</span>
      ) : offline ? (
        <span style={{ fontSize: 12, color: "#A3ADC4" }}>ForgeDM is closed</span>
      ) : (
        <>
          <span className="mono" style={{ fontWeight: 600, fontSize: 14 }}>{st && st.speed > 0 ? fmtSpeed(st.speed) : "Idle"}</span>
          <span style={{ fontSize: 12, color: "#A3ADC4", whiteSpace: "nowrap" }}>{st ? `${st.active} active` : ""}</span>
        </>
      )}
    </div>
  );
}
