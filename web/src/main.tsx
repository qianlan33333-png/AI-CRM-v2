import React from "react";
import { createRoot } from "react-dom/client";

export function App() {
  return (
    <main aria-labelledby="app-title">
      <h1 id="app-title">AI-CRM</h1>
      <p>Web 应用骨架已就绪</p>
    </main>
  );
}

export function mount(root: HTMLElement) {
  createRoot(root).render(<App />);
}

if (typeof document !== "undefined") {
  const root = document.getElementById("root");
  if (!root) throw new Error("AI-CRM web root is missing");
  mount(root);
}
