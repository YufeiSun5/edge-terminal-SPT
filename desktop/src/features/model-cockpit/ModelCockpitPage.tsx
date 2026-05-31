import { useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  Activity,
  Building2,
  Clock3,
  Cpu,
  Droplets,
  Gauge,
  HeartPulse,
  Package,
  Power,
  ShieldCheck,
  Thermometer,
  Volume2,
  Wind,
  Zap,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import * as THREE from 'three'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'
import { getActiveDetectionRuns, getCurrentDetectionRun, getRealtimeVariables } from '@/features/edge-status/api'
import type { DetectionRunStandardItem, TagSnapshot } from '@/shared/api/types'
import './model-cockpit.css'

type TopCardConfig = {
  labelKey: string
  hintKey: string
  value: string
  icon: LucideIcon
}

type MetricCardConfig = {
  labelKey: string
  matchNames: string[]
  value: string
  unit: string
  limitMin: number
  limitMax: number
  optimization: number
  icon: LucideIcon
  points: number[]
}

const TOP_CARDS: TopCardConfig[] = [
  { labelKey: 'modelCockpit.cards.model', hintKey: 'modelCockpit.cards.modelHint', value: 'CRAC-EDGE', icon: Package },
  { labelKey: 'modelCockpit.cards.serial', hintKey: 'modelCockpit.cards.serialHint', value: 'EDGE-3D-01', icon: Cpu },
  { labelKey: 'modelCockpit.cards.customer', hintKey: 'modelCockpit.cards.customerHint', value: 'Spindle Lab', icon: Building2 },
  { labelKey: 'modelCockpit.cards.duration', hintKey: 'modelCockpit.cards.durationHint', value: '10:13:30', icon: Clock3 },
  { labelKey: 'modelCockpit.cards.result', hintKey: 'modelCockpit.cards.resultHint', value: 'OK', icon: ShieldCheck },
]

const METRIC_CARDS: MetricCardConfig[] = [
  {
    labelKey: 'station.metrics.tempOut',
    matchNames: ['吹出口温度', 'Outlet temperature', 'tempOut', 'out_temp'],
    value: '31.1',
    unit: 'degC',
    limitMin: 20,
    limitMax: 55,
    optimization: 91,
    icon: Thermometer,
    points: [29, 30, 28, 31, 30, 32, 31, 30, 30, 31],
  },
  {
    labelKey: 'station.metrics.humidIn',
    matchNames: ['吸入口湿度', 'Inlet humidity', 'humidIn', 'in_humidity'],
    value: '24.2',
    unit: '%RH',
    limitMin: 20,
    limitMax: 60,
    optimization: 86,
    icon: Droplets,
    points: [22, 25, 21, 24, 26, 23, 23, 25, 22, 26],
  },
  {
    labelKey: 'station.metrics.pressure',
    matchNames: ['系统压力', 'Pressure', 'pressure'],
    value: '100',
    unit: 'kPa',
    limitMin: 100,
    limitMax: 150,
    optimization: 78,
    icon: Gauge,
    points: [96, 101, 98, 100, 99, 97, 98, 101, 100, 102],
  },
  {
    labelKey: 'station.metrics.windIn',
    matchNames: ['吸入风量', 'Inlet airflow', 'windIn', 'airflow'],
    value: '128',
    unit: 'm3/h',
    limitMin: 120,
    limitMax: 160,
    optimization: 88,
    icon: Wind,
    points: [112, 128, 119, 126, 130, 124, 123, 129, 118, 132],
  },
  {
    labelKey: 'station.metrics.noise',
    matchNames: ['设备噪音', 'Noise', 'noise'],
    value: '45.3',
    unit: 'dB',
    limitMin: 40,
    limitMax: 75,
    optimization: 93,
    icon: Volume2,
    points: [42, 46, 43, 45, 44, 47, 43, 42, 44, 46],
  },
  {
    labelKey: 'station.metrics.vibration',
    matchNames: ['震动位移', 'Vibration', 'vibration'],
    value: '0.12',
    unit: 'mm',
    limitMin: 0.05,
    limitMax: 2,
    optimization: 96,
    icon: Activity,
    points: [0.08, 0.12, 0.1, 0.13, 0.09, 0.11, 0.14, 0.1, 0.12, 0.13],
  },
  {
    labelKey: 'station.metrics.power',
    matchNames: ['设备功率', 'Power', 'power'],
    value: '2.4',
    unit: 'kW',
    limitMin: 2,
    limitMax: 4.5,
    optimization: 82,
    icon: Power,
    points: [2.1, 2.4, 2.2, 2.3, 2.5, 2.2, 2.2, 2.4, 2.3, 2.5],
  },
  {
    labelKey: 'station.metrics.compressorSuctionTemp',
    matchNames: ['压缩机吸入管温度', 'Compressor suction', 'suction'],
    value: '12.6',
    unit: 'degC',
    limitMin: 10,
    limitMax: 15,
    optimization: 90,
    icon: Zap,
    points: [11.8, 12.4, 12.3, 12.8, 12.6, 12.1, 12.2, 12.0, 12.5, 12.7],
  },
  {
    labelKey: 'station.metrics.compressorDischargeTemp',
    matchNames: ['压缩机吐出口温度', 'Compressor discharge', 'discharge'],
    value: '85.3',
    unit: 'degC',
    limitMin: 70,
    limitMax: 90,
    optimization: 84,
    icon: Activity,
    points: [72, 76, 80, 82, 84, 86, 85, 84, 83, 85],
  },
  {
    labelKey: 'station.metrics.condenserOutletTemp',
    matchNames: ['冷凝器出口温度', 'Condenser outlet', 'condenser'],
    value: '35.2',
    unit: 'degC',
    limitMin: 30,
    limitMax: 45,
    optimization: 87,
    icon: HeartPulse,
    points: [32, 33, 34, 35, 36, 35, 34, 35, 35, 36],
  },
]

const COCKPIT_MODEL_PATH = '/models/cockpit/new-shaded.glb'

export function ModelCockpitPage() {
  const { t } = useTranslation()
  const lightPos = useCockpitLight()

  useEffect(() => {
    const resetScroll = () => {
      window.scrollTo({ top: 0, left: 0 })
      document.documentElement.scrollTop = 0
      document.body.scrollTop = 0
      document.querySelector<HTMLElement>('.workbench-content')?.scrollTo({ top: 0, left: 0 })
    }
    resetScroll()
    const frame = window.requestAnimationFrame(resetScroll)
    const timer = window.setTimeout(resetScroll, 120)
    return () => {
      window.cancelAnimationFrame(frame)
      window.clearTimeout(timer)
    }
  }, [])

  const activeRunsQuery = useQuery({
    queryKey: ['model-cockpit', 'active-runs'],
    queryFn: getActiveDetectionRuns,
    refetchInterval: 3000,
    retry: false,
  })
  const activeProjectId = activeRunsQuery.data?.[0]?.project_id
  const realtimeQuery = useQuery({
    queryKey: ['model-cockpit', 'realtime', activeProjectId ?? 'all'],
    queryFn: () => getRealtimeVariables(activeProjectId ? { project_id: activeProjectId } : {}),
    refetchInterval: 2000,
    retry: false,
  })
  const currentRunQuery = useQuery({
    queryKey: ['model-cockpit', 'current-run', activeProjectId],
    queryFn: () => getCurrentDetectionRun(activeProjectId!),
    enabled: activeProjectId !== undefined,
    refetchInterval: 5000,
    retry: false,
  })
  const metricCards = useMemo(
    () => resolveMetricCards(METRIC_CARDS, realtimeQuery.data ?? [], currentRunQuery.data?.standard_items ?? []),
    [currentRunQuery.data?.standard_items, realtimeQuery.data],
  )
  const primaryMetrics = metricCards.slice(0, 8)
  const detailMetrics = [metricCards[0], metricCards[3], metricCards[4], metricCards[7]].filter(Boolean)

  return (
    <div
      className="model-cockpit-page"
      style={{ '--light-x': `${lightPos.pageX}px`, '--light-y': `${lightPos.pageY}px` } as CSSProperties}
    >
      <CockpitDynamicBackground />

      <header className="cockpit-title-row">
        <div className="cockpit-title-bar">
          <CockpitTitleFrame />
          <span>{t('modelCockpit.title')}</span>
          <small>{t('modelCockpit.eyebrow')}</small>
        </div>
      </header>

      <section className="cockpit-top-row" aria-label={t('modelCockpit.title')}>
        {TOP_CARDS.map((card, index) => (
          <CockpitGlassPanel className={`cockpit-top-card cockpit-card-tone-${index}`} key={card.labelKey} title={t(card.labelKey)}>
            <TopCardContent card={card} />
          </CockpitGlassPanel>
        ))}
      </section>

      <section className="cockpit-left-grid" aria-label={t('modelCockpit.status.telemetry')}>
        {primaryMetrics.map((metric, index) => (
          <CockpitGlassPanel className={`cockpit-metric-card cockpit-card-tone-${index % 5}`} key={metric.labelKey} title={t(metric.labelKey)}>
            <MetricCardContent metric={metric} />
          </CockpitGlassPanel>
        ))}
      </section>

      <main className="cockpit-center-stage" aria-label={t('modelCockpit.title')}>
        <CockpitModelStage />
      </main>

      <aside className="cockpit-right-stack" aria-label={t('modelCockpit.status.telemetry')}>
        {detailMetrics.map((metric, index) => (
          <CockpitGlassPanel className={`cockpit-side-card cockpit-card-tone-${index + 2}`} key={metric.labelKey} title={t(metric.labelKey)}>
            <MetricDetailContent metric={metric} />
          </CockpitGlassPanel>
        ))}
      </aside>
    </div>
  )
}

type ResolvedMetricCard = MetricCardConfig & {
  status: 'OK' | 'NG'
}

function normalizeMetricName(value: string) {
  return value.toLowerCase().replace(/[\s_()（）/%℃°.-]/g, '')
}

function matchesMetricName(metric: MetricCardConfig, names: Array<string | undefined>) {
  const targets = metric.matchNames.map(normalizeMetricName)
  return names.some((name) => {
    if (!name) return false
    const normalized = normalizeMetricName(name)
    return targets.some((target) => normalized.includes(target) || target.includes(normalized))
  })
}

function resolveMetricCards(metrics: MetricCardConfig[], snapshots: TagSnapshot[], standardItems: DetectionRunStandardItem[]): ResolvedMetricCard[] {
  return metrics.map((metric) => {
    const snapshot = snapshots.find((item) =>
      matchesMetricName(metric, [item.var_name, item.display_name, item.display_name_en, item.display_name_ja, item.source_path]),
    )
    const standardItem = standardItems.find((item) => {
      if (snapshot && item.var_id === snapshot.var_id) return true
      return matchesMetricName(metric, [item.var_name, item.display_name, item.display_name_en, item.display_name_ja])
    })
    const numericValue = snapshot && !snapshot.is_string ? snapshot.value : Number(metric.value)
    const limitMin = pickNumber(standardItem?.limit_l, standardItem?.limit_ll, metric.limitMin)
    const limitMax = pickNumber(standardItem?.limit_h, standardItem?.limit_hh, metric.limitMax)
    const unit = standardItem?.unit || metric.unit
    const value = Number.isFinite(numericValue) ? formatMetricValue(numericValue, standardItem?.decimal_places ?? inferDecimals(metric.value)) : metric.value
    const status = Number.isFinite(numericValue) && numericValue >= limitMin && numericValue <= limitMax ? 'OK' : 'NG'
    const optimization = Number.isFinite(numericValue) ? getOptimizationRate(numericValue, limitMin, limitMax) : metric.optimization
    return { ...metric, limitMin, limitMax, optimization, status, unit, value }
  })
}

function pickNumber(...values: Array<number | null | undefined>) {
  return values.find((value): value is number => typeof value === 'number' && Number.isFinite(value)) ?? 0
}

function inferDecimals(value: string) {
  return value.includes('.') ? Math.min(value.split('.')[1]?.length ?? 0, 2) : 0
}

function formatMetricValue(value: number, decimals: number) {
  return value.toFixed(Math.max(0, Math.min(decimals, 2)))
}

function getOptimizationRate(value: number, limitMin: number, limitMax: number) {
  if (limitMax <= limitMin) return 0
  const center = (limitMin + limitMax) / 2
  const halfRange = (limitMax - limitMin) / 2
  return Math.round(Math.max(0, Math.min(100, 100 - (Math.abs(value - center) / halfRange) * 100)))
}

function CockpitTitleFrame() {
  return (
    <svg className="cockpit-title-frame" viewBox="0 0 720 130" aria-hidden="true" preserveAspectRatio="none">
      <defs>
        <linearGradient id="cockpit-title-fill" x1="0" x2="1" y1="0" y2="1">
          <stop offset="0" stopColor="#ffffff" stopOpacity="0.68" />
          <stop offset="0.54" stopColor="#f4f8fb" stopOpacity="0.42" />
          <stop offset="1" stopColor="#dce8f5" stopOpacity="0.2" />
        </linearGradient>
        <linearGradient id="cockpit-title-stroke" x1="0" x2="1">
          <stop offset="0" stopColor="#9bb8d8" stopOpacity="0" />
          <stop offset="0.18" stopColor="#dbe8f6" />
          <stop offset="0.52" stopColor="#88a9cf" />
          <stop offset="0.84" stopColor="#edf5ff" />
          <stop offset="1" stopColor="#9bb8d8" stopOpacity="0" />
        </linearGradient>
        <filter id="cockpit-title-glow" x="-20%" y="-40%" width="140%" height="180%">
          <feGaussianBlur stdDeviation="1.6" result="blur" />
          <feMerge>
            <feMergeNode in="blur" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
      </defs>
      <path
        d="M84 10 H636 L608 48 L566 48 L542 78 H178 L154 48 H112 Z"
        fill="url(#cockpit-title-fill)"
        opacity="0.86"
        filter="url(#cockpit-title-glow)"
      />
      <path d="M84 10 H636 L608 48 L566 48 L542 78 H178 L154 48 H112 Z" fill="none" stroke="url(#cockpit-title-stroke)" strokeWidth="2" />
      <path d="M116 20 H604 M170 86 H550" fill="none" stroke="#9bb8d8" strokeDasharray="7 8" strokeOpacity="0.2" />
      <circle cx="130" cy="78" r="3" fill="#90a9c4" opacity="0.36" />
      <circle cx="590" cy="78" r="3" fill="#90a9c4" opacity="0.36" />
    </svg>
  )
}

function CockpitGlassPanel({ children, className, title }: { children?: ReactNode; className?: string; title?: ReactNode }) {
  return (
    <section className={['cockpit-glass-panel', 'glass-panel', className].filter(Boolean).join(' ')}>
      <div className="cockpit-panel-title">
        <span>{title}</span>
      </div>
      <div className="cockpit-panel-body">{children}</div>
    </section>
  )
}

function TopCardContent({ card }: { card: TopCardConfig }) {
  const { t } = useTranslation()
  const Icon = card.icon
  return (
    <div className="cockpit-top-content">
      <div className="cockpit-top-icon">
        <Icon aria-hidden="true" strokeWidth={1.8} />
      </div>
      <div className="cockpit-top-copy">
        <strong>{card.value}</strong>
        <small>{t(card.hintKey)}</small>
      </div>
    </div>
  )
}

function MetricCardContent({ metric }: { metric: ResolvedMetricCard }) {
  const { t } = useTranslation()
  const Icon = metric.icon
  return (
    <div className="cockpit-metric-content">
      <Icon className="cockpit-metric-icon" aria-hidden="true" strokeWidth={1.9} />
      <span className={metric.status === 'OK' ? 'cockpit-metric-status ok' : 'cockpit-metric-status ng'}>{metric.status}</span>
      <strong>
        {metric.value} <span>{formatUnit(metric.unit)}</span>
      </strong>
      <div className="cockpit-metric-range">
        <span>
          {t('modelCockpit.metric.limit')}: {formatLimit(metric.limitMin)}-{formatLimit(metric.limitMax)} {formatUnit(metric.unit)}
        </span>
        <span>{t('modelCockpit.metric.optimization', { value: metric.optimization })}</span>
      </div>
    </div>
  )
}

function MetricDetailContent({ metric }: { metric: ResolvedMetricCard }) {
  const { t } = useTranslation()
  const Icon = metric.icon
  return (
    <div className="cockpit-detail-content">
      <div className="cockpit-detail-heading">
        <span className="cockpit-detail-icon">
          <Icon aria-hidden="true" strokeWidth={1.85} />
        </span>
        <span className={metric.status === 'OK' ? 'cockpit-metric-status ok' : 'cockpit-metric-status ng'}>{metric.status}</span>
      </div>
      <strong>
        {metric.value}
        <span>{formatUnit(metric.unit)}</span>
      </strong>
      <dl>
        <div>
          <dt>{t('modelCockpit.metric.limit')}</dt>
          <dd>
            {formatLimit(metric.limitMin)} - {formatLimit(metric.limitMax)} {formatUnit(metric.unit)}
          </dd>
        </div>
        <div>
          <dt>{t('modelCockpit.metric.optimizationLabel')}</dt>
          <dd>{metric.optimization}%</dd>
        </div>
      </dl>
      <div className="cockpit-detail-progress" style={{ '--progress': `${metric.optimization}%` } as CSSProperties} />
    </div>
  )
}

function formatLimit(value: number) {
  if (Math.abs(value) >= 10 || Number.isInteger(value)) return String(Math.round(value * 10) / 10)
  return value.toFixed(2)
}

function formatUnit(unit: string) {
  if (unit === 'degC') return '°C'
  if (unit === 'm3/h') return 'm³/h'
  return unit
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
      <span className="cockpit-bg-surface cockpit-bg-surface-1" />
      <span className="cockpit-bg-surface cockpit-bg-surface-2" />
      <span className="cockpit-bg-light" />
    </div>
  )
}

function CockpitModelStage() {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const modelRef = useRef<THREE.Group | null>(null)

  useEffect(() => {
    const host = hostRef.current
    if (!host) return

    const scene = new THREE.Scene()
    const camera = new THREE.PerspectiveCamera(28, 1, 0.1, 100)
    camera.position.set(3.1, -0.32, 6.85)
    camera.lookAt(0, -2.9, 0)

    const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true })
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
    renderer.setClearColor(0x000000, 0)
    renderer.outputColorSpace = THREE.SRGBColorSpace
    renderer.toneMapping = THREE.LinearToneMapping
    renderer.toneMappingExposure = 1
    host.appendChild(renderer.domElement)

    const controls = new OrbitControls(camera, renderer.domElement)
    controls.enableDamping = true
    controls.enablePan = false
    controls.enableZoom = false
    controls.target.set(0, -2.9, 0)
    controls.autoRotate = true
    controls.autoRotateSpeed = 0.25

    scene.add(new THREE.AmbientLight(0xffffff, 0.9))
    scene.add(new THREE.HemisphereLight(0xffffff, 0xb5c8dc, 1.15))

    const keyLight = new THREE.DirectionalLight(0xffffff, 2.8)
    keyLight.position.set(3.6, 5.2, 4.8)
    scene.add(keyLight)

    const fillLight = new THREE.DirectionalLight(0xeaf4ff, 1.35)
    fillLight.position.set(-3.2, 2.4, 3.2)
    scene.add(fillLight)

    const rimLight = new THREE.DirectionalLight(0xddeeff, 1.1)
    rimLight.position.set(-2.8, 3.2, -4.5)
    scene.add(rimLight)

    const modelFloorY = -4.35

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

    let alive = true

    const loader = new GLTFLoader()
    loader.load(COCKPIT_MODEL_PATH, (gltf) => {
      if (!alive) return
      const model = gltf.scene
      const box = new THREE.Box3().setFromObject(model)
      const size = box.getSize(new THREE.Vector3())
      const center = box.getCenter(new THREE.Vector3())
      const maxAxis = Math.max(size.x, size.y, size.z) || 1
      model.position.sub(center)
      model.scale.setScalar(3.18 / maxAxis)
      model.rotation.y = -0.35
      const scaledBox = new THREE.Box3().setFromObject(model)
      model.position.y += modelFloorY - scaledBox.min.y
      model.traverse((node) => {
        if (node instanceof THREE.Mesh) {
          node.castShadow = true
          node.receiveShadow = true
          if (node.material instanceof THREE.MeshStandardMaterial) {
            node.material.envMapIntensity = 0.65
            node.material.roughness = Math.min(node.material.roughness + 0.18, 0.82)
            node.material.metalness *= 0.72
          }
        }
      })
      modelRef.current = model
      scene.add(model)
      fluidGroup.visible = false
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
      renderer.render(scene, camera)
      frame = window.requestAnimationFrame(animate)
    }
    frame = window.requestAnimationFrame(animate)

    return () => {
      window.cancelAnimationFrame(frame)
      alive = false
      observer.disconnect()
      controls.dispose()
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
  }, [])

  return <div ref={hostRef} className="cockpit-model-stage" />
}
