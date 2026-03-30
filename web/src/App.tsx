import { Activity, Shield, Zap } from "lucide-react";

const cards = [
  { label: "Requests / min", value: "12,430", icon: Activity },
  { label: "Error Rate", value: "0.38%", icon: Zap },
  { label: "Protected Routes", value: "184", icon: Shield },
];

export function App() {
  return (
    <div className="page-shell">
      <header className="hero">
        <p className="eyebrow">API CERBERUS</p>
        <h1>Operational cockpit for secure API traffic.</h1>
        <p className="subtitle">
          React + TypeScript dashboard shell is ready. Data hooks and feature pages can plug into this layout.
        </p>
      </header>

      <main className="grid">
        {cards.map((card) => {
          const Icon = card.icon;
          return (
            <article key={card.label} className="stat-card">
              <div className="stat-top">
                <span>{card.label}</span>
                <Icon size={16} />
              </div>
              <strong>{card.value}</strong>
            </article>
          );
        })}
      </main>
    </div>
  );
}

