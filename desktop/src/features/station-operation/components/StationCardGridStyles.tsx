export function StationCardGridStyles() {
  return (
    <style>{`
      .station-page .grid-scroll-container::-webkit-scrollbar {
        display: none;
      }
      .station-page .grid-scroll-container {
        -ms-overflow-style: none;
        scrollbar-width: none;
        container-type: size;
      }

      @keyframes bounceDown {
        0%, 100% { transform: translateY(0); }
        50% { transform: translateY(3px); }
      }
      @keyframes bounceUp {
        0%, 100% { transform: translateY(0); }
        50% { transform: translateY(-3px); }
      }

      .station-page .glass-panel {
        background-color: rgba(255, 255, 255, 0.25);
        backdrop-filter: blur(24px) saturate(150%);
        -webkit-backdrop-filter: blur(24px) saturate(150%);
        box-shadow:
          0 8px 32px rgba(0, 0, 0, 0.08),
          inset 0 0 0 1px rgba(255, 255, 255, 0.1);
        position: relative;
        border: 1px solid rgba(255, 255, 255, 0.05);
        transform: scale(1);
        cursor: grab;
        transition:
          transform 0.4s cubic-bezier(0.2, 0.8, 0.2, 1),
          box-shadow 0.4s cubic-bezier(0.2, 0.8, 0.2, 1),
          background-color 0.4s;
      }

      .station-page .glass-panel.dragging {
        transform: scale(1.05);
        box-shadow:
          0 32px 64px rgba(0,0,0,0.2),
          inset 0 0 0 1px rgba(255,255,255,0.4);
        background-color: rgba(255, 255, 255, 0.35);
        cursor: grabbing;
      }

      .station-page .glass-panel::after {
        content: "";
        position: absolute;
        top: -1px;
        left: -1px;
        right: -1px;
        bottom: -1px;
        border-radius: inherit;
        padding: 1px;
        background: radial-gradient(
          600px circle at var(--mouse-x, 50%) var(--mouse-y, 50%),
          rgba(255, 220, 150, 1) 0%,
          rgba(255, 255, 255, 0.9) 10%,
          rgba(255, 255, 255, 0.1) 40%,
          rgba(255, 255, 255, 0) 60%
        );
        -webkit-mask: linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0);
        -webkit-mask-composite: xor;
        mask-composite: exclude;
        pointer-events: none;
        z-index: 10;
      }
    `}</style>
  )
}
