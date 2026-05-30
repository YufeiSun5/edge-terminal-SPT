import { useEffect, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import * as THREE from 'three'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'
import './model-cockpit.css'

const TOP_CARD_KEYS = [
  'modelCockpit.cards.model',
  'modelCockpit.cards.serial',
  'modelCockpit.cards.customer',
  'modelCockpit.cards.duration',
  'modelCockpit.cards.result',
]

const LEFT_CARD_KEYS = [
  'station.metrics.tempOut',
  'station.metrics.humidIn',
  'station.metrics.pressure',
  'station.metrics.windIn',
  'station.metrics.noise',
  'station.metrics.vibration',
  'station.metrics.power',
  'station.metrics.compressorSuctionTemp',
  'station.metrics.compressorDischargeTemp',
  'station.metrics.condenserOutletTemp',
]

const RIGHT_CARD_KEYS = ['modelCockpit.charts.temperature', 'modelCockpit.charts.humidity']

const MODEL_HOTSPOTS = [
  { key: 'outTemp', labelKey: 'station.metrics.tempOut', value: '31.1 degC', anchor: new THREE.Vector3(-0.38, 0.22, 0.2) },
  { key: 'wind', labelKey: 'station.metrics.windIn', value: '128 m3/h', anchor: new THREE.Vector3(-0.4, -0.18, 0.16) },
  { key: 'noise', labelKey: 'station.metrics.noise', value: '45.3 dB', anchor: new THREE.Vector3(0.18, -0.05, 0.22) },
  { key: 'pressure', labelKey: 'station.metrics.pressure', value: '100 kPa', anchor: new THREE.Vector3(0.38, 0.1, 0.18) },
]

export function ModelCockpitPage() {
  const { t } = useTranslation()
  const lightPos = useCockpitLight()

  return (
    <div
      className="model-cockpit-page"
      style={{ '--light-x': `${lightPos.pageX}px`, '--light-y': `${lightPos.pageY}px` } as CSSProperties}
    >
      <CockpitStationGlassStyles />
      <CockpitDynamicBackground />

      <header className="cockpit-title-row">
        <div className="cockpit-title-bar">
          <span>{t('modelCockpit.title')}</span>
          <small>{t('modelCockpit.eyebrow')}</small>
        </div>
      </header>

      <section className="cockpit-top-row" aria-label={t('modelCockpit.title')}>
        {TOP_CARD_KEYS.map((labelKey, index) => (
          <CockpitGlassPanel className={`cockpit-top-card cockpit-card-tone-${index}`} key={labelKey} title={t(labelKey)} />
        ))}
      </section>

      <section className="cockpit-left-grid" aria-label={t('modelCockpit.status.telemetry')}>
        {LEFT_CARD_KEYS.map((labelKey, index) => (
          <CockpitGlassPanel className={`cockpit-metric-card cockpit-card-tone-${index % 5}`} key={labelKey} title={t(labelKey)} />
        ))}
      </section>

      <main className="cockpit-center-stage" aria-label={t('modelCockpit.title')}>
        <CockpitModelStage lightPos={lightPos} />
      </main>

      <aside className="cockpit-right-stack" aria-label={t('modelCockpit.status.telemetry')}>
        {RIGHT_CARD_KEYS.map((labelKey, index) => (
          <CockpitGlassPanel className={`cockpit-side-card cockpit-card-tone-${index + 2}`} key={labelKey} title={t(labelKey)} />
        ))}
      </aside>
    </div>
  )
}

function CockpitStationGlassStyles() {
  return (
    <style>{`
      .model-cockpit-page .glass-panel {
        background-color: rgba(255, 255, 255, 0.25);
        backdrop-filter: blur(24px) saturate(150%);
        -webkit-backdrop-filter: blur(24px) saturate(150%);
        box-shadow:
          0 8px 32px rgba(0, 0, 0, 0.08),
          inset 0 0 0 1px rgba(255, 255, 255, 0.1);
        position: relative;
        border: 1px solid rgba(255, 255, 255, 0.05);
        transform: scale(1);
        transition:
          transform 0.4s cubic-bezier(0.2, 0.8, 0.2, 1),
          box-shadow 0.4s cubic-bezier(0.2, 0.8, 0.2, 1),
          background-color 0.4s;
      }

      .model-cockpit-page .glass-panel::after {
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

function CockpitGlassPanel({ className, title }: { className?: string; title?: ReactNode }) {
  return (
    <section className={['cockpit-glass-panel', 'glass-panel', 'metric-card', className].filter(Boolean).join(' ')}>
      <div className="cockpit-panel-title">
        <span>{title}</span>
      </div>
      <div className="cockpit-panel-body" />
    </section>
  )
}

type CockpitLightPosition = {
  pageX: number
  pageY: number
  viewportX: number
  viewportY: number
}

function useCockpitLight() {
  const [lightPos, setLightPos] = useState<CockpitLightPosition>({
    pageX: window.innerWidth / 2,
    pageY: window.innerHeight / 2,
    viewportX: window.innerWidth / 2,
    viewportY: window.innerHeight / 2,
  })

  useEffect(() => {
    let frame = 0
    const cycle = 20000
    const start = Date.now()

    const animate = () => {
      const pageRect = document.querySelector<HTMLElement>('.model-cockpit-page')?.getBoundingClientRect()
      if (!pageRect) {
        frame = window.requestAnimationFrame(animate)
        return
      }
      const progress = ((Date.now() - start) % cycle) / cycle
      const angle = progress * Math.PI * 2
      const x = pageRect.left + pageRect.width / 2 + Math.cos(angle) * pageRect.width * 0.35
      const y = pageRect.top + pageRect.height / 2 + Math.sin(angle) * pageRect.height * 0.25
      setLightPos({ pageX: x - pageRect.left, pageY: y - pageRect.top, viewportX: x, viewportY: y })
      document.querySelectorAll<HTMLElement>('.model-cockpit-page .glass-panel').forEach((panel) => {
        const rect = panel.getBoundingClientRect()
        panel.style.setProperty('--mouse-x', `${x - rect.left}px`)
        panel.style.setProperty('--mouse-y', `${y - rect.top}px`)
      })
      frame = window.requestAnimationFrame(animate)
    }

    frame = window.requestAnimationFrame(animate)
    return () => window.cancelAnimationFrame(frame)
  }, [])

  return lightPos
}

function CockpitDynamicBackground() {
  return (
    <div className="cockpit-dynamic-background" aria-hidden="true">
      <span className="cockpit-orb cockpit-orb-1" />
      <span className="cockpit-orb cockpit-orb-2" />
      <span className="cockpit-orb cockpit-orb-3" />
    </div>
  )
}

function CockpitModelStage({ lightPos }: { lightPos: CockpitLightPosition }) {
  const { t } = useTranslation()
  const hostRef = useRef<HTMLDivElement | null>(null)
  const modelRef = useRef<THREE.Group | null>(null)
  const keyLightRef = useRef<THREE.DirectionalLight | null>(null)
  const fillLightRef = useRef<THREE.PointLight | null>(null)
  const [callouts, setCallouts] = useState<
    Array<{ key: string; label: string; value: string; x: number; y: number; visible: boolean; side: 'left' | 'right' }>
  >([])

  useEffect(() => {
    const host = hostRef.current
    if (!host) return

    const scene = new THREE.Scene()
    const camera = new THREE.PerspectiveCamera(34, 1, 0.1, 100)
    camera.position.set(3.35, 1.85, 6.9)
    camera.lookAt(0, -0.78, 0)

    const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true })
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
    renderer.setClearColor(0x000000, 0)
    renderer.outputColorSpace = THREE.SRGBColorSpace
    renderer.toneMapping = THREE.ACESFilmicToneMapping
    renderer.toneMappingExposure = 1.18
    host.appendChild(renderer.domElement)

    const controls = new OrbitControls(camera, renderer.domElement)
    controls.enableDamping = true
    controls.enablePan = false
    controls.enableZoom = false
    controls.target.set(0, -0.78, 0)
    controls.autoRotate = true
    controls.autoRotateSpeed = 0.25

    scene.add(new THREE.HemisphereLight(0xbddfff, 0x203650, 1.4))

    const keyLight = new THREE.DirectionalLight(0xffe3ad, 4.2)
    keyLight.position.set(4, 5, 5)
    scene.add(keyLight)
    keyLightRef.current = keyLight

    const fillLight = new THREE.PointLight(0x7ec5ff, 2.2, 12)
    fillLight.position.set(-3, 2, 3)
    scene.add(fillLight)
    fillLightRef.current = fillLight

    const platform = new THREE.Mesh(
      new THREE.RingGeometry(1.1, 2.0, 128),
      new THREE.MeshBasicMaterial({ color: 0x8ee8ff, transparent: true, opacity: 0.13, side: THREE.DoubleSide }),
    )
    platform.rotation.x = -Math.PI / 2
    platform.position.y = -1.72
    scene.add(platform)

    const fluidGroup = new THREE.Group()
    fluidGroup.visible = false
    scene.add(fluidGroup)
    const fluidParticles: THREE.Mesh[] = []
    const fluidCurve = new THREE.CatmullRomCurve3([
      new THREE.Vector3(-0.9, -0.84, 0.42),
      new THREE.Vector3(-0.45, -0.35, 0.34),
      new THREE.Vector3(0.12, -0.12, 0.24),
      new THREE.Vector3(0.58, 0.22, 0.3),
      new THREE.Vector3(0.78, 0.64, 0.2),
    ])
    const suctionCurve = new THREE.CatmullRomCurve3([
      new THREE.Vector3(-0.78, 0.38, -0.08),
      new THREE.Vector3(-0.18, 0.24, 0.08),
      new THREE.Vector3(0.42, 0.02, 0.16),
      new THREE.Vector3(0.82, -0.22, 0.28),
    ])
    ;[
      { curve: fluidCurve, color: 0x7ff4ff, opacity: 0.6 },
      { curve: suctionCurve, color: 0xffd266, opacity: 0.5 },
    ].forEach(({ curve, color, opacity }) => {
      const tube = new THREE.Mesh(
        new THREE.TubeGeometry(curve, 96, 0.014, 10, false),
        new THREE.MeshBasicMaterial({ color, transparent: true, opacity, depthTest: false }),
      )
      tube.renderOrder = 3
      fluidGroup.add(tube)
    })
    for (let index = 0; index < 16; index += 1) {
      const particle = new THREE.Mesh(
        new THREE.SphereGeometry(index % 2 ? 0.028 : 0.022, 16, 16),
        new THREE.MeshBasicMaterial({ color: index % 3 ? 0x88f7ff : 0xffd266, transparent: true, opacity: 0.82, depthTest: false }),
      )
      particle.renderOrder = 4
      particle.userData.offset = index / 16
      particle.userData.curve = index % 3 === 0 ? suctionCurve : fluidCurve
      fluidParticles.push(particle)
      fluidGroup.add(particle)
    }

    const raycaster = new THREE.Raycaster()
    const modelMeshes: THREE.Object3D[] = []
    let frameIndex = 0
    let alive = true

    const loader = new GLTFLoader()
    loader.load('/models/edge-air-conditioner.glb', (gltf) => {
      const model = gltf.scene
      const box = new THREE.Box3().setFromObject(model)
      const size = box.getSize(new THREE.Vector3())
      const center = box.getCenter(new THREE.Vector3())
      const maxAxis = Math.max(size.x, size.y, size.z) || 1
      model.position.sub(center)
      model.scale.setScalar(3.05 / maxAxis)
      model.rotation.y = -0.35
      const scaledBox = new THREE.Box3().setFromObject(model)
      model.position.y += platform.position.y - scaledBox.min.y + 0.02
      model.traverse((node) => {
        if (node instanceof THREE.Mesh) {
          modelMeshes.push(node)
          node.castShadow = true
          node.receiveShadow = true
          if (node.material instanceof THREE.MeshStandardMaterial) {
            node.material.envMapIntensity = 1.25
            node.material.roughness = Math.min(node.material.roughness + 0.08, 0.72)
          }
        }
      })
      modelRef.current = model
      scene.add(model)
      fluidGroup.visible = true
      fluidGroup.position.copy(model.position)
      fluidGroup.rotation.copy(model.rotation)
      fluidGroup.scale.setScalar(1.18)
    })

    const resize = () => {
      const rect = host.getBoundingClientRect()
      const width = Math.max(1, rect.width)
      const height = Math.max(1, rect.height)
      renderer.setSize(width, height, false)
      camera.aspect = width / height
      camera.updateProjectionMatrix()
    }

    const observer = new ResizeObserver(resize)
    observer.observe(host)
    resize()

    let frame = 0
    const animate = () => {
      const elapsed = performance.now() / 1000
      controls.update()
      if (modelRef.current) {
        fluidGroup.position.copy(modelRef.current.position)
        fluidGroup.rotation.copy(modelRef.current.rotation)
        fluidGroup.rotation.y += Math.sin(elapsed * 0.7) * 0.03
      }
      fluidParticles.forEach((particle) => {
        const curve = particle.userData.curve as THREE.CatmullRomCurve3
        const point = curve.getPoint((particle.userData.offset + elapsed * 0.12) % 1)
        particle.position.copy(point)
        particle.scale.setScalar(0.7 + Math.sin(elapsed * 5 + particle.userData.offset * 10) * 0.22)
      })
      if (modelRef.current && frameIndex % 3 === 0) {
        const hostRect = host.getBoundingClientRect()
        const nextCallouts = MODEL_HOTSPOTS.map((hotspot) => {
          const world = modelRef.current!.localToWorld(hotspot.anchor.clone())
          const projected = world.clone().project(camera)
          const direction = world.clone().sub(camera.position).normalize()
          raycaster.set(camera.position, direction)
          const intersections = raycaster.intersectObjects(modelMeshes, false)
          const distanceToAnchor = camera.position.distanceTo(world)
          const blocked = intersections.some((hit) => hit.distance < distanceToAnchor - 0.08)
          return {
            key: hotspot.key,
            label: t(hotspot.labelKey),
            value: hotspot.value,
            x: ((projected.x + 1) / 2) * hostRect.width,
            y: ((1 - projected.y) / 2) * hostRect.height,
            visible: projected.z > -1 && projected.z < 1 && !blocked,
            side: projected.x < 0 ? ('left' as const) : ('right' as const),
          }
        })
        if (alive) setCallouts(nextCallouts)
      }
      frameIndex += 1
      renderer.render(scene, camera)
      frame = window.requestAnimationFrame(animate)
    }
    frame = window.requestAnimationFrame(animate)

    return () => {
      window.cancelAnimationFrame(frame)
      alive = false
      observer.disconnect()
      controls.dispose()
      keyLightRef.current = null
      fillLightRef.current = null
      modelRef.current = null
      scene.traverse((node) => {
        if (node instanceof THREE.Mesh) {
          node.geometry.dispose()
          if (Array.isArray(node.material)) node.material.forEach((material) => material.dispose())
          else node.material.dispose()
        }
      })
      renderer.dispose()
      host.removeChild(renderer.domElement)
    }
  }, [t])

  useEffect(() => {
    const host = hostRef.current
    const keyLight = keyLightRef.current
    const fillLight = fillLightRef.current
    if (!host || !keyLight || !fillLight) return
    const rect = host.getBoundingClientRect()
    const localX = ((lightPos.viewportX - rect.left) / Math.max(rect.width, 1) - 0.5) * 7
    const localY = (0.5 - (lightPos.viewportY - rect.top) / Math.max(rect.height, 1)) * 4.5
    keyLight.position.set(localX, 3.8 + localY * 0.35, 4.5)
    fillLight.position.set(localX * 0.72, 1.4 + localY * 0.25, 2.8)
  }, [lightPos])

  return (
    <div ref={hostRef} className="cockpit-model-stage">
      <div className="cockpit-model-callouts" aria-hidden="true">
        {callouts.map((callout) => (
          <div
            className={callout.visible ? `cockpit-model-callout ${callout.side}` : `cockpit-model-callout ${callout.side} hidden`}
            key={callout.key}
            style={{ transform: `translate(${callout.x}px, ${callout.y}px)` }}
          >
            <strong>{callout.value}</strong>
            <span>{callout.label}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
