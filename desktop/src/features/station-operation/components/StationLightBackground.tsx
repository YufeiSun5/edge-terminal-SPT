import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'

type StationLightBackgroundProps = {
  scopeClassName?: string
}

export function StationLightBackground({ scopeClassName = 'station-page' }: StationLightBackgroundProps) {
  const [lightPos, setLightPos] = useState({ x: '50vw', y: '50vh' })

  useEffect(() => {
    let frame = 0
    const cycle = 20000
    const start = Date.now()

    const animate = () => {
      const progress = ((Date.now() - start) % cycle) / cycle
      const angle = progress * Math.PI * 2
      const x = window.innerWidth / 2 + Math.cos(angle) * window.innerWidth * 0.35
      const y = window.innerHeight / 2 + Math.sin(angle) * window.innerHeight * 0.25

      setLightPos({ x: `${x}px`, y: `${y}px` })
      document.querySelectorAll<HTMLElement>(`.${scopeClassName} .glass-panel`).forEach((panel) => {
        const rect = panel.getBoundingClientRect()
        panel.style.setProperty('--mouse-x', `${x - rect.left}px`)
        panel.style.setProperty('--mouse-y', `${y - rect.top}px`)
      })

      frame = window.requestAnimationFrame(animate)
    }

    frame = window.requestAnimationFrame(animate)
    return () => window.cancelAnimationFrame(frame)
  }, [scopeClassName])

  return (
    <>
      <div
        className="ambient-background"
        style={{ '--light-x': lightPos.x, '--light-y': lightPos.y } as CSSProperties}
      >
        <div className="silk-orb orb-1" />
        <div className="silk-orb orb-2" />
        <div className="silk-orb orb-3" />
      </div>
      <style>{`
        @keyframes silk-drift-1 {
          0%, 100% { transform: translate(0, 0) scale(1); }
          50% { transform: translate(15vw, 10vh) scale(1.05); }
        }
        @keyframes silk-drift-2 {
          0%, 100% { transform: translate(0, 0) scale(1); }
          50% { transform: translate(-15vw, -10vh) scale(0.95); }
        }
        @keyframes silk-drift-3 {
          0%, 100% { transform: translate(0, 0) scale(1); }
          50% { transform: translate(10vw, -15vh) scale(1.1); }
        }

        .${scopeClassName} .ambient-background {
          position: absolute;
          top: 0;
          left: 0;
          right: 0;
          bottom: 0;
          z-index: 0;
          overflow: hidden;
          pointer-events: none;
        }

        .${scopeClassName} .silk-orb {
          position: absolute;
          border-radius: 50%;
          filter: blur(80px);
          opacity: 0.95;
          z-index: 0;
          mix-blend-mode: normal;
        }

        .${scopeClassName} .orb-1 {
          top: -10%;
          left: -10%;
          width: 90vw;
          height: 90vw;
          background: radial-gradient(circle, rgba(22, 119, 255, 0.85) 0%, rgba(22, 119, 255, 0) 70%);
          animation: silk-drift-1 18s ease-in-out infinite;
        }

        .${scopeClassName} .orb-2 {
          bottom: -20%;
          right: -10%;
          width: 100vw;
          height: 100vw;
          background: radial-gradient(circle, rgba(0, 80, 180, 0.75) 0%, rgba(0, 80, 180, 0) 70%);
          animation: silk-drift-2 22s ease-in-out infinite;
        }

        .${scopeClassName} .orb-3 {
          top: var(--light-y);
          left: var(--light-x);
          transform: translate(-50%, -50%);
          width: 80vw;
          height: 80vw;
          background: radial-gradient(circle, rgba(255, 180, 50, 0.45) 0%, rgba(255, 180, 50, 0) 60%);
        }
      `}</style>
    </>
  )
}
