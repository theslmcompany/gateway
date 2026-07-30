import type { ReactNode } from "react";
import { Link } from "react-router";

// ─── SLM Company intro page (theslmcompany.com /) ───────────────────────────────
// Company-level landing. The Gateway product lives at /gateway.

interface Pillar {
  tag: string;
  title: string;
  body: string;
}

const PILLARS: Pillar[] = [
  {
    tag: "INFRA",
    title: "LLM infrastructure",
    body: "Gateways, routing, and control planes that sit between your applications and every model provider.",
  },
  {
    tag: "GOVERNANCE",
    title: "Guardrails & observability",
    body: "PII masking, policy enforcement, and full-fidelity tracing for every request your agents make.",
  },
  {
    tag: "RESEARCH",
    title: "Applied LLM tooling",
    body: "Practical tooling on top of large and small language models — built for teams shipping to production.",
  },
];

const Logo = (): ReactNode => (
  <div className="flex items-center gap-3">
    <div
      className="w-8 h-8 rounded-lg flex items-center justify-center"
      style={{
        background: "rgba(34,211,238,0.08)",
        border: "1px solid rgba(34,211,238,0.2)",
      }}
    >
      <svg viewBox="0 0 32 32" fill="none" className="w-5 h-5">
        <circle
          cx="16"
          cy="16"
          r="14"
          stroke="#22d3ee"
          strokeWidth="1.5"
          strokeOpacity="0.2"
        />
        <circle
          cx="16"
          cy="16"
          r="8"
          stroke="#22d3ee"
          strokeWidth="1.5"
          strokeOpacity="0.4"
        />
        <circle cx="16" cy="16" r="3" fill="#22d3ee" />
        <path
          d="M16 4V16L24 24"
          stroke="#22d3ee"
          strokeWidth="2"
          strokeLinecap="round"
        />
      </svg>
    </div>
    <span className="font-mono font-black text-base tracking-[0.2em] text-white uppercase">
      The <span className="text-cyan-400">SLM</span> Company
    </span>
  </div>
);

export default function CompanyIntro(): ReactNode {
  return (
    <div
      className="min-h-screen text-slate-200"
      style={{ backgroundColor: "#070c12" }}
    >
      {/* ── Nav ── */}
      <nav
        className="sticky top-0 z-50 border-b"
        style={{
          backgroundColor: "rgba(7,12,18,0.85)",
          borderColor: "rgba(34,211,238,0.1)",
          backdropFilter: "blur(12px)",
        }}
      >
        <div className="w-full px-6 lg:px-12 xl:px-24 h-14 flex items-center justify-between">
          <Logo />
          <div className="flex items-center gap-4">
            <Link
              to="/gateway"
              className="text-xs font-mono font-bold tracking-widest px-4 py-2 rounded transition-colors"
              style={{
                background: "rgba(34,211,238,0.1)",
                border: "1px solid rgba(34,211,238,0.3)",
                color: "#22d3ee",
              }}
            >
              THE GATEWAY
            </Link>
          </div>
        </div>
      </nav>

      {/* ── Hero ── */}
      <section className="relative overflow-hidden pt-28 pb-20 px-6 lg:px-12 xl:px-24">
        <div className="relative max-w-4xl mx-auto text-center">
          <h1 className="text-4xl sm:text-5xl lg:text-[4.5rem] font-black tracking-tight leading-none mb-6 text-white">
            Infrastructure for the
            <br />
            <span style={{ color: "#22d3ee" }}>language-model era.</span>
          </h1>

          <p className="text-lg sm:text-xl text-slate-400 max-w-2xl mx-auto mb-12 leading-relaxed">
            The SLM Company builds the control plane between your applications
            and every language model — routing, guardrails, and observability
            for teams running LLMs in production.
          </p>

          <div className="flex flex-col sm:flex-row gap-4 justify-center">
            <Link
              to="/gateway"
              className="text-sm font-mono font-bold tracking-widest px-6 py-3 rounded transition-colors"
              style={{ background: "#22d3ee", color: "#04121a" }}
            >
              EXPLORE THE GATEWAY →
            </Link>
            <a
              href="mailto:bchaitanya15@gmail.com"
              className="text-sm font-mono font-bold tracking-widest px-6 py-3 rounded transition-colors"
              style={{
                background: "rgba(34,211,238,0.08)",
                border: "1px solid rgba(34,211,238,0.25)",
                color: "#22d3ee",
              }}
            >
              GET IN TOUCH
            </a>
          </div>
        </div>
      </section>

      {/* ── Pillars ── */}
      <section className="px-6 lg:px-12 xl:px-24 pb-24">
        <div className="max-w-5xl mx-auto grid gap-6 sm:grid-cols-3">
          {PILLARS.map((p) => (
            <div
              key={p.tag}
              className="rounded-xl p-6 text-left"
              style={{
                background: "rgba(34,211,238,0.03)",
                border: "1px solid rgba(34,211,238,0.12)",
              }}
            >
              <span className="text-xs font-mono font-bold tracking-widest text-cyan-600">
                {p.tag}
              </span>
              <h3 className="mt-3 text-lg font-bold text-white">{p.title}</h3>
              <p className="mt-2 text-sm text-slate-400 leading-relaxed">
                {p.body}
              </p>
            </div>
          ))}
        </div>
      </section>

      {/* ── Gateway callout ── */}
      <section className="px-6 lg:px-12 xl:px-24 pb-28">
        <div
          className="max-w-4xl mx-auto rounded-2xl p-10 text-center"
          style={{
            background: "rgba(34,211,238,0.04)",
            border: "1px solid rgba(34,211,238,0.15)",
          }}
        >
          <span className="text-xs font-mono font-bold tracking-widest text-cyan-600">
            FLAGSHIP PRODUCT
          </span>
          <h2 className="mt-3 text-3xl font-black text-white">AI Gateway</h2>
          <p className="mt-3 text-slate-400 max-w-2xl mx-auto leading-relaxed">
            One URL change puts air traffic control in front of your AI agents —
            routing traffic, enforcing guardrails, masking PII, and recording
            every span.
          </p>
          <Link
            to="/gateway"
            className="inline-block mt-8 text-sm font-mono font-bold tracking-widest px-6 py-3 rounded transition-colors"
            style={{
              background: "rgba(34,211,238,0.1)",
              border: "1px solid rgba(34,211,238,0.3)",
              color: "#22d3ee",
            }}
          >
            VIEW PRODUCT →
          </Link>
        </div>
      </section>

      {/* ── Footer ── */}
      <footer
        className="border-t px-6 lg:px-12 xl:px-24 py-10"
        style={{ borderColor: "rgba(34,211,238,0.1)" }}
      >
        <div className="max-w-5xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-4">
          <Logo />
          <span className="text-xs font-mono text-slate-600">
            © {2025} The SLM Company · theslmcompany.com
          </span>
        </div>
      </footer>
    </div>
  );
}
