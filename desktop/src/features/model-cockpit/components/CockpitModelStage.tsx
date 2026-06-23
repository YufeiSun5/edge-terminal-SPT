import { useEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import { Button, InputNumber, Segmented, Slider } from 'antd'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'

const COCKPIT_MODEL_PATH = '/models/cockpit/new-shaded.glb'
const MODEL_SWING_SPEED = 0.38
const MODEL_SWING_TOTAL_DEGREES = 85
const MODEL_SWING_AMPLITUDE = THREE.MathUtils.degToRad(MODEL_SWING_TOTAL_DEGREES / 2)
const MODEL_SWING_SEGMENT_MS = (Math.PI / (2 * MODEL_SWING_SPEED)) * 1000
const MODEL_RETURN_DELAY_MS = 15_000
const MODEL_RETURN_DURATION_MS = 6_000
const MODEL_LABEL_INTERVAL_MS = 2600
const FLOW_TEXTURE_SPEED_MULTIPLIER = 6.4

type FlowKey = 'water' | 'air' | 'heat'
type ModelLabelKey = 'humidifier' | 'compressor' | 'inlet' | 'heatExchanger' | 'fan' | 'outlet'
type EditorMode = 'flow' | 'label'

type FlowPoint = {
  x: number
  y: number
  z: number
}

type FlowEditorPoints = Record<FlowKey, FlowPoint[]>
type ModelLabelPoints = Record<ModelLabelKey, FlowPoint>

const FLOW_KEYS: FlowKey[] = ['water', 'air', 'heat']
const MODEL_LABEL_KEYS: ModelLabelKey[] = ['humidifier', 'compressor', 'inlet', 'heatExchanger', 'fan', 'outlet']

const DEFAULT_FLOW_POINTS: FlowEditorPoints = {
  water: [
    { x: 0, y: 0.26, z: 0.62 },
    { x: 0, y: 0.26, z: 0.45 },
    { x: 0, y: 0.41, z: 0.45 },
    { x: 0, y: 0.41, z: -0.57 },
    { x: 0, y: 0.19, z: -0.57 },
    { x: 0, y: 0.19, z: 0.62 },
  ],
  air: [
    { x: 0, y: 0.94, z: -0.64 },
    { x: 0, y: 0.94, z: -0.23 },
    { x: 0, y: 1.83, z: -0.23 },
    { x: 0, y: 1.83, z: -0.01 },
    { x: 0, y: 2.71, z: -0.01 },
  ],
  heat: [
    { x: 0, y: 0.54, z: 0.67 },
    { x: 0, y: 0.54, z: 0.42 },
    { x: 0, y: 1.05, z: 0.42 },
    { x: 0, y: 1.05, z: 0.67 },
  ],
}

const FLOW_EDITOR_META: Record<FlowKey, { label: string; tone: string; description: string }> = {
  water: { label: '蓝色冷却水', tone: '#1677ff', description: '底座横向冷却水和右侧上行段' },
  air: { label: '绿色气流', tone: '#22c55e', description: '顶部风筒进入、中部折向和右侧流路' },
  heat: { label: '红色热流', tone: '#ef4444', description: '压缩机侧前部横向热流' },
}

const DEFAULT_MODEL_LABEL_POINTS: ModelLabelPoints = {
  humidifier: { x: 0, y: 0.29, z: 0.04 },
  compressor: { x: 0.01, y: 0.71, z: 0.22 },
  inlet: { x: 0, y: 0.95, z: -0.75 },
  heatExchanger: { x: 0.02, y: 1.2, z: 0.18 },
  fan: { x: 0.02, y: 1.84, z: 0.07 },
  outlet: { x: 0, y: 2.7, z: -0.02 },
}

const MODEL_LABEL_META: Record<ModelLabelKey, { label: string; oldOffset: string; tone: string }> = {
  humidifier: { label: '加湿器', oldOffset: '旧版 2D: top 73%, left 60%', tone: '#22c55e' },
  compressor: { label: '压缩机', oldOffset: '旧版 2D: top 70%, left 38%', tone: '#1677ff' },
  inlet: { label: '吸入口', oldOffset: '旧版 2D: top 64%, left 79%', tone: '#0d9488' },
  heatExchanger: { label: '热交换器', oldOffset: '旧版 2D: top 55%, left 65%', tone: '#ef4444' },
  fan: { label: '风机', oldOffset: '旧版 2D: top 31%, left 47%', tone: '#6366f1' },
  outlet: { label: '吹出口', oldOffset: '旧版 2D: top 6%, left 48.5%', tone: '#f59e0b' },
}

const DEFAULT_POINT_LABELS: Record<FlowKey, string[]> = {
  water: ['入口', '内收段', '上折点', '后侧横向', '下折点', '回流出口'],
  air: ['底部入口', '前段横向', '竖向上行', '上部横向', '顶部出口'],
  heat: ['热流入口', '内收段', '上行段', '回流出口'],
}

type CockpitModelStageProps = {
  showFlowEditor?: boolean
}

type FlowPathConfig = {
  key: FlowKey
  curve: THREE.Curve<THREE.Vector3>
  coreColor: number
  glowColor: number
  radius: number
  speed: number
  opacity: number
}

type FlowAnimationData = {
  texture: THREE.Texture
  speed: number
}

function cloneFlowPoints(points: FlowEditorPoints): FlowEditorPoints {
  return {
    water: points.water.map((point) => ({ ...point })),
    air: points.air.map((point) => ({ ...point })),
    heat: points.heat.map((point) => ({ ...point })),
  }
}

function cloneLabelPoints(points: ModelLabelPoints): ModelLabelPoints {
  return {
    humidifier: { ...points.humidifier },
    compressor: { ...points.compressor },
    inlet: { ...points.inlet },
    heatExchanger: { ...points.heatExchanger },
    fan: { ...points.fan },
    outlet: { ...points.outlet },
  }
}

function toVector3(point: FlowPoint) {
  return new THREE.Vector3(point.x, point.y, point.z)
}

function toVector3Points(points: FlowPoint[]) {
  return points.map(toVector3)
}

function addCurveSegment(curve: THREE.CurvePath<THREE.Vector3>, start: THREE.Vector3, end: THREE.Vector3) {
  if (start.distanceToSquared(end) > 0.000001) {
    curve.add(new THREE.LineCurve3(start.clone(), end.clone()))
  }
}

function createPolylineCurve(points: THREE.Vector3[], cornerRadius = 0.09) {
  const curve = new THREE.CurvePath<THREE.Vector3>()
  if (points.length < 3 || cornerRadius <= 0) {
    for (let index = 0; index < points.length - 1; index += 1) {
      addCurveSegment(curve, points[index], points[index + 1])
    }
    return curve
  }

  let segmentStart = points[0].clone()
  for (let index = 1; index < points.length - 1; index += 1) {
    const previous = points[index - 1]
    const corner = points[index]
    const next = points[index + 1]
    const previousDistance = corner.distanceTo(previous)
    const nextDistance = corner.distanceTo(next)
    const radius = Math.min(cornerRadius, previousDistance * 0.42, nextDistance * 0.42)

    if (radius <= 0.001) {
      addCurveSegment(curve, segmentStart, corner)
      segmentStart = corner.clone()
      continue
    }

    const beforeCorner = corner.clone().lerp(previous, radius / previousDistance)
    const afterCorner = corner.clone().lerp(next, radius / nextDistance)
    addCurveSegment(curve, segmentStart, beforeCorner)
    curve.add(new THREE.QuadraticBezierCurve3(beforeCorner, corner.clone(), afterCorner))
    segmentStart = afterCorner
  }
  addCurveSegment(curve, segmentStart, points[points.length - 1])
  return curve
}

function createFlowMaterial(color: number, opacity: number, blending: THREE.Blending = THREE.NormalBlending) {
  return new THREE.MeshBasicMaterial({
    color,
    transparent: true,
    opacity,
    depthTest: false,
    depthWrite: false,
    blending,
  })
}

function createFlowStripeTexture(color: number) {
  const canvas = document.createElement('canvas')
  canvas.width = 256
  canvas.height = 32
  const context = canvas.getContext('2d')
  if (!context) throw new Error('flow texture canvas context unavailable')

  context.clearRect(0, 0, canvas.width, canvas.height)
  const colorStyle = `#${color.toString(16).padStart(6, '0')}`
  const gradient = context.createLinearGradient(0, 0, canvas.width, 0)
  gradient.addColorStop(0, 'rgba(255,255,255,0)')
  gradient.addColorStop(0.18, colorStyle)
  gradient.addColorStop(0.5, colorStyle)
  gradient.addColorStop(0.76, 'rgba(255,255,255,0.62)')
  gradient.addColorStop(1, 'rgba(255,255,255,0)')
  context.fillStyle = gradient
  context.shadowBlur = 10
  context.shadowColor = colorStyle
  context.beginPath()
  context.roundRect(18, 8, 104, 16, 8)
  context.fill()

  const texture = new THREE.CanvasTexture(canvas)
  texture.wrapS = THREE.RepeatWrapping
  texture.wrapT = THREE.ClampToEdgeWrapping
  texture.repeat.set(7, 1)
  texture.needsUpdate = true
  return texture
}

function createFlowTextureMaterial(texture: THREE.Texture, opacity: number) {
  return new THREE.MeshBasicMaterial({
    map: texture,
    transparent: true,
    opacity,
    depthTest: false,
    depthWrite: false,
    blending: THREE.NormalBlending,
  })
}

function addFlowPath(group: THREE.Group, config: FlowPathConfig, animations: FlowAnimationData[]) {
  const shadowTube = new THREE.Mesh(
    new THREE.TubeGeometry(config.curve, 128, config.radius * 1.35, 14, false),
    createFlowMaterial(config.glowColor, config.opacity * 0.32),
  )
  shadowTube.renderOrder = 4
  group.add(shadowTube)

  const glowTube = new THREE.Mesh(
    new THREE.TubeGeometry(config.curve, 128, config.radius * 2.7, 18, false),
    createFlowMaterial(config.glowColor, config.opacity * 0.18, THREE.AdditiveBlending),
  )
  glowTube.renderOrder = 5
  group.add(glowTube)

  const coreTube = new THREE.Mesh(
    new THREE.TubeGeometry(config.curve, 128, config.radius, 12, false),
    createFlowMaterial(config.coreColor, config.opacity * 0.56),
  )
  coreTube.renderOrder = 6
  group.add(coreTube)

  const highlightTube = new THREE.Mesh(
    new THREE.TubeGeometry(config.curve, 128, config.radius * 0.38, 8, false),
    createFlowMaterial(config.coreColor, config.opacity * 0.18),
  )
  highlightTube.renderOrder = 7
  group.add(highlightTube)

  const stripeTexture = createFlowStripeTexture(config.coreColor)
  const flowBandTube = new THREE.Mesh(
    new THREE.TubeGeometry(config.curve, 192, config.radius * 1.06, 14, false),
    createFlowTextureMaterial(stripeTexture, 0.92),
  )
  flowBandTube.renderOrder = 9
  group.add(flowBandTube)
  animations.push({ texture: stripeTexture, speed: config.speed })
}

function disposeFlowObjects(group: THREE.Group) {
  group.traverse((node) => {
    if (node instanceof THREE.Mesh) {
      node.geometry.dispose()
      if (Array.isArray(node.material)) {
        node.material.forEach((material) => {
          material.map?.dispose()
          material.dispose()
        })
      } else {
        node.material.map?.dispose()
        node.material.dispose()
      }
    }
  })
  group.clear()
}

function formatNumber(value: number) {
  return Number(value.toFixed(2))
}

function formatFlowPointCode(points: FlowPoint[]) {
  return points.map((point) => `new THREE.Vector3(${formatNumber(point.x)}, ${formatNumber(point.y)}, ${formatNumber(point.z)}),`).join('\n')
}

function formatLabelPointCode(key: ModelLabelKey, point: FlowPoint) {
  return `${key}: { x: ${formatNumber(point.x)}, y: ${formatNumber(point.y)}, z: ${formatNumber(point.z)} },`
}

function getInsertedPoint(points: FlowPoint[]) {
  const lastPoint = points[points.length - 1] ?? { x: 0, y: 0, z: 0 }
  const previousPoint = points[points.length - 2]
  if (!previousPoint) return { ...lastPoint, x: formatNumber(lastPoint.x + 0.2) }
  return {
    x: formatNumber(lastPoint.x + (lastPoint.x - previousPoint.x) * 0.5),
    y: formatNumber(lastPoint.y + (lastPoint.y - previousPoint.y) * 0.5),
    z: formatNumber(lastPoint.z + (lastPoint.z - previousPoint.z) * 0.5),
  }
}

function getPointLabel(flowKey: FlowKey, index: number) {
  return DEFAULT_POINT_LABELS[flowKey][index] ?? `节点 ${index}`
}

export function CockpitModelStage({ showFlowEditor = false }: CockpitModelStageProps) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const modelRef = useRef<THREE.Group | null>(null)
  const labelElementsRef = useRef<Record<ModelLabelKey, HTMLDivElement | null>>({
    humidifier: null,
    compressor: null,
    inlet: null,
    heatExchanger: null,
    fan: null,
    outlet: null,
  })
  const updateFlowPathRef = useRef<((key: FlowKey, points: FlowPoint[]) => void) | null>(null)
  const initialFlowPointsRef = useRef<FlowEditorPoints>(cloneFlowPoints(DEFAULT_FLOW_POINTS))
  const labelPointsRef = useRef<ModelLabelPoints>(cloneLabelPoints(DEFAULT_MODEL_LABEL_POINTS))
  const showFlowEditorRef = useRef(showFlowEditor)
  const rotationPausedRef = useRef(false)
  const [flowEditorPoints, setFlowEditorPoints] = useState<FlowEditorPoints>(() => cloneFlowPoints(DEFAULT_FLOW_POINTS))
  const [labelEditorPoints, setLabelEditorPoints] = useState<ModelLabelPoints>(() => cloneLabelPoints(DEFAULT_MODEL_LABEL_POINTS))
  const [editorMode, setEditorMode] = useState<EditorMode>('flow')
  const [activeFlowKey, setActiveFlowKey] = useState<FlowKey>('water')
  const [activeLabelKey, setActiveLabelKey] = useState<ModelLabelKey>('humidifier')
  const [copyState, setCopyState] = useState<'idle' | 'copied'>('idle')
  const [rotationPaused, setRotationPaused] = useState(false)
  const activeFlowPoints = flowEditorPoints[activeFlowKey]
  const activeFlowMeta = FLOW_EDITOR_META[activeFlowKey]
  const activeLabelPoint = labelEditorPoints[activeLabelKey]
  const activeLabelMeta = MODEL_LABEL_META[activeLabelKey]
  const flowOptions = useMemo(
    () => FLOW_KEYS.map((key) => ({ label: FLOW_EDITOR_META[key].label, value: key })),
    [],
  )
  const labelOptions = useMemo(
    () => MODEL_LABEL_KEYS.map((key) => ({ label: MODEL_LABEL_META[key].label, value: key })),
    [],
  )
  const activePointCode = useMemo(() => formatFlowPointCode(activeFlowPoints), [activeFlowPoints])
  const activeLabelPointCode = useMemo(
    () => formatLabelPointCode(activeLabelKey, activeLabelPoint),
    [activeLabelKey, activeLabelPoint],
  )

  useEffect(() => {
    showFlowEditorRef.current = showFlowEditor
  }, [showFlowEditor])

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
    controls.autoRotate = false
    const swingOffset = new THREE.Vector3().copy(camera.position).sub(controls.target)
    const swingSpherical = new THREE.Spherical().setFromVector3(swingOffset)
    const baseSpherical = swingSpherical.clone()
    let swingStartedAt = performance.now()
    let cameraMode: 'swing' | 'manual' | 'settling' | 'returning' = 'swing'
    let returnAvailableAt = 0
    let returnStartedAt = 0
    const returnFromSpherical = baseSpherical.clone()

    const handleControlStart = () => {
      cameraMode = 'manual'
    }
    const handleControlEnd = () => {
      cameraMode = 'settling'
      returnAvailableAt = performance.now() + MODEL_RETURN_DELAY_MS
    }
    controls.addEventListener('start', handleControlStart)
    controls.addEventListener('end', handleControlEnd)

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
    const flowAnimations: FlowAnimationData[] = []
    // 流体路径坐标说明：
    // x 控制左右位置，负数向模型左侧，正数向模型右侧。
    // y 控制高度，数值越大越靠上，数值越小越靠近底座。
    // z 控制前后深度，数值越大越靠屏幕前方，数值越小越往模型内部。
    // 每个 Vector3 都是一个拐点，流体会按数组顺序从第一个点流向最后一个点。
    const flowPathPoints: Record<FlowKey, THREE.Vector3[]> = {
      water: toVector3Points(initialFlowPointsRef.current.water),
      air: toVector3Points(initialFlowPointsRef.current.air),
      heat: toVector3Points(initialFlowPointsRef.current.heat),
    }
    const flowPaths: FlowPathConfig[] = [
      {
        key: 'water',
        curve: createPolylineCurve(flowPathPoints.water),
        coreColor: 0x1d8cff,
        glowColor: 0x0758d8,
        radius: 0.03,
        speed: 0.085,
        opacity: 0.9,
      },
      {
        key: 'air',
        curve: createPolylineCurve(flowPathPoints.air),
        coreColor: 0x22c55e,
        glowColor: 0x047857,
        radius: 0.027,
        speed: 0.105,
        opacity: 0.82,
      },
      {
        key: 'heat',
        curve: createPolylineCurve(flowPathPoints.heat),
        coreColor: 0xff334e,
        glowColor: 0xb91c1c,
        radius: 0.028,
        speed: 0.12,
        opacity: 0.86,
      },
    ]
    const renderFlowPaths = () => {
      flowAnimations.splice(0, flowAnimations.length)
      disposeFlowObjects(fluidGroup)
      flowPaths.forEach((path) => addFlowPath(fluidGroup, path, flowAnimations))
    }
    renderFlowPaths()

    updateFlowPathRef.current = (key, points) => {
      const flowPath = flowPaths.find((path) => path.key === key)
      if (!flowPath) return
      flowPathPoints[key] = toVector3Points(points)
      flowPath.curve = createPolylineCurve(flowPathPoints[key])
      renderFlowPaths()
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

    const labelFrame = new THREE.Object3D()
    const labelWorldPosition = new THREE.Vector3()
    const labelScreenPosition = new THREE.Vector3()
    const labelCameraPosition = new THREE.Vector3()

    let frame = 0
    const animate = () => {
      const now = performance.now()
      const elapsed = now / 1000
      if (!rotationPausedRef.current) {
        if (cameraMode === 'swing') {
          const swingElapsed = (now - swingStartedAt) % (MODEL_SWING_SEGMENT_MS * 4)
          const segmentIndex = Math.floor(swingElapsed / MODEL_SWING_SEGMENT_MS)
          const segmentProgress = (swingElapsed % MODEL_SWING_SEGMENT_MS) / MODEL_SWING_SEGMENT_MS
          const easedProgress = 0.5 - Math.cos(segmentProgress * Math.PI) / 2
          const segmentTargets = [
            [0, MODEL_SWING_AMPLITUDE],
            [MODEL_SWING_AMPLITUDE, 0],
            [0, -MODEL_SWING_AMPLITUDE],
            [-MODEL_SWING_AMPLITUDE, 0],
          ] as const
          const [fromThetaOffset, toThetaOffset] = segmentTargets[segmentIndex] ?? segmentTargets[0]
          swingSpherical.radius = baseSpherical.radius
          swingSpherical.phi = baseSpherical.phi
          swingSpherical.theta = baseSpherical.theta + THREE.MathUtils.lerp(fromThetaOffset, toThetaOffset, easedProgress)
          swingOffset.setFromSpherical(swingSpherical)
          camera.position.copy(controls.target).add(swingOffset)
          camera.lookAt(controls.target)
        } else if (cameraMode === 'settling' && now >= returnAvailableAt) {
          returnFromSpherical.setFromVector3(swingOffset.copy(camera.position).sub(controls.target))
          returnStartedAt = now
          cameraMode = 'returning'
        } else if (cameraMode === 'returning') {
          const progress = Math.min(1, (now - returnStartedAt) / MODEL_RETURN_DURATION_MS)
          const easedProgress = 1 - Math.pow(1 - progress, 3)
          swingSpherical.radius = THREE.MathUtils.lerp(returnFromSpherical.radius, baseSpherical.radius, easedProgress)
          swingSpherical.phi = THREE.MathUtils.lerp(returnFromSpherical.phi, baseSpherical.phi, easedProgress)
          swingSpherical.theta = THREE.MathUtils.lerp(returnFromSpherical.theta, baseSpherical.theta, easedProgress)
          swingOffset.setFromSpherical(swingSpherical)
          camera.position.copy(controls.target).add(swingOffset)
          camera.lookAt(controls.target)
          if (progress >= 1) {
            cameraMode = 'swing'
            swingStartedAt = now
          }
        }
      }
      controls.update()
      if (modelRef.current) {
        fluidGroup.position.copy(modelRef.current.position)
        fluidGroup.rotation.copy(modelRef.current.rotation)
        fluidGroup.rotation.y += Math.sin(elapsed * 0.7) * 0.03

        labelFrame.position.copy(modelRef.current.position)
        labelFrame.rotation.copy(modelRef.current.rotation)
        labelFrame.scale.setScalar(1.18)
        labelFrame.updateMatrixWorld(true)
        const rect = host.getBoundingClientRect()
        const activeOrderedLabel = MODEL_LABEL_KEYS[Math.floor((elapsed * 1000) / MODEL_LABEL_INTERVAL_MS) % MODEL_LABEL_KEYS.length]
        for (const key of MODEL_LABEL_KEYS) {
          const element = labelElementsRef.current[key]
          if (!element) continue
          labelWorldPosition.copy(toVector3(labelPointsRef.current[key])).applyMatrix4(labelFrame.matrixWorld)
          labelCameraPosition.copy(labelWorldPosition).applyMatrix4(camera.matrixWorldInverse)
          labelScreenPosition.copy(labelWorldPosition).project(camera)
          const projected =
            labelCameraPosition.z < 0 &&
            Math.abs(labelScreenPosition.x) <= 1.15 &&
            Math.abs(labelScreenPosition.y) <= 1.15 &&
            (showFlowEditorRef.current || key === activeOrderedLabel)
          if (!projected) {
            element.style.display = 'none'
            continue
          }
          const x = ((labelScreenPosition.x + 1) / 2) * rect.width
          const y = ((1 - labelScreenPosition.y) / 2) * rect.height
          element.style.display = 'inline-flex'
          element.style.transform = `translate3d(${x}px, ${y}px, 0) translate(-50%, -50%)`
        }
      }
      flowAnimations.forEach(({ texture, speed }) => {
        texture.offset.x = -((elapsed * speed * FLOW_TEXTURE_SPEED_MULTIPLIER) % 1)
      })
      renderer.render(scene, camera)
      frame = window.requestAnimationFrame(animate)
    }
    frame = window.requestAnimationFrame(animate)

    return () => {
      window.cancelAnimationFrame(frame)
      alive = false
      updateFlowPathRef.current = null
      observer.disconnect()
      controls.removeEventListener('start', handleControlStart)
      controls.removeEventListener('end', handleControlEnd)
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

  const updateFlowPoint = (flowKey: FlowKey, index: number, axis: keyof FlowPoint, rawValue: number) => {
    const value = formatNumber(rawValue)
    setCopyState('idle')
    setFlowEditorPoints((current) => {
      const next = {
        ...current,
        [flowKey]: current[flowKey].map((point, pointIndex) => (pointIndex === index ? { ...point, [axis]: value } : point)),
      }
      updateFlowPathRef.current?.(flowKey, next[flowKey])
      return next
    })
  }

  const addFlowPoint = () => {
    setCopyState('idle')
    setFlowEditorPoints((current) => {
      const nextPoints = [...current[activeFlowKey], getInsertedPoint(current[activeFlowKey])]
      const next = { ...current, [activeFlowKey]: nextPoints }
      updateFlowPathRef.current?.(activeFlowKey, nextPoints)
      return next
    })
  }

  const removeFlowPoint = (index: number) => {
    setCopyState('idle')
    setFlowEditorPoints((current) => {
      if (current[activeFlowKey].length <= 2) return current
      const nextPoints = current[activeFlowKey].filter((_, pointIndex) => pointIndex !== index)
      const next = { ...current, [activeFlowKey]: nextPoints }
      updateFlowPathRef.current?.(activeFlowKey, nextPoints)
      return next
    })
  }

  const resetActiveFlowPoints = () => {
    const nextPoints = DEFAULT_FLOW_POINTS[activeFlowKey].map((point) => ({ ...point }))
    setCopyState('idle')
    setFlowEditorPoints((current) => ({ ...current, [activeFlowKey]: nextPoints }))
    updateFlowPathRef.current?.(activeFlowKey, nextPoints)
  }

  const updateLabelPoint = (labelKey: ModelLabelKey, axis: keyof FlowPoint, rawValue: number) => {
    const value = formatNumber(rawValue)
    setCopyState('idle')
    setLabelEditorPoints((current) => {
      const next = {
        ...current,
        [labelKey]: { ...current[labelKey], [axis]: value },
      }
      labelPointsRef.current = cloneLabelPoints(next)
      return next
    })
  }

  const resetActiveLabelPoint = () => {
    const nextPoint = { ...DEFAULT_MODEL_LABEL_POINTS[activeLabelKey] }
    setCopyState('idle')
    setLabelEditorPoints((current) => {
      const next = { ...current, [activeLabelKey]: nextPoint }
      labelPointsRef.current = cloneLabelPoints(next)
      return next
    })
  }

  const copyActiveFlowPointCode = async () => {
    await navigator.clipboard.writeText(activePointCode)
    setCopyState('copied')
  }

  const copyActiveLabelPointCode = async () => {
    await navigator.clipboard.writeText(activeLabelPointCode)
    setCopyState('copied')
  }

  const selectFlowKey = (key: FlowKey) => {
    setActiveFlowKey(key)
    setCopyState('idle')
  }

  const selectLabelKey = (key: ModelLabelKey) => {
    setActiveLabelKey(key)
    setCopyState('idle')
  }

  const toggleRotationPaused = () => {
    setRotationPaused((current) => {
      const next = !current
      rotationPausedRef.current = next
      return next
    })
  }

  return (
    <div className="cockpit-model-stage">
      <div ref={hostRef} className="cockpit-model-stage-canvas" />
      {MODEL_LABEL_KEYS.map((key) => (
        <div
          key={key}
          className="model-stage-label"
          ref={(node) => {
            labelElementsRef.current[key] = node
          }}
          style={{
            '--label-tone': MODEL_LABEL_META[key].tone,
          } as CSSProperties & { '--label-tone': string }}
        >
          <span />
          <strong>{MODEL_LABEL_META[key].label}</strong>
        </div>
      ))}
      {showFlowEditor ? (
        <aside className="flow-editor-panel" aria-label="模型路径和标签调试">
          <header>
            <div>
              <span>{editorMode === 'flow' ? `${activeFlowKey} path` : `${activeLabelKey} label`}</span>
              <strong style={{ color: editorMode === 'flow' ? activeFlowMeta.tone : activeLabelMeta.tone }}>
                {editorMode === 'flow' ? activeFlowMeta.label : activeLabelMeta.label}
              </strong>
            </div>
            <div className="flow-editor-header-actions">
              <Button size="small" onClick={toggleRotationPaused}>
                {rotationPaused ? '恢复旋转' : '停止旋转'}
              </Button>
              <Button size="small" onClick={editorMode === 'flow' ? resetActiveFlowPoints : resetActiveLabelPoint}>
                重置
              </Button>
            </div>
          </header>
          <div className="flow-editor-tabs" role="tablist" aria-label="选择调试类型">
            <Segmented
              block
              options={[
                { label: '流体路径', value: 'flow' },
                { label: '模型标签', value: 'label' },
              ]}
              value={editorMode}
              onChange={(value) => {
                setEditorMode(value as EditorMode)
                setCopyState('idle')
              }}
            />
          </div>
          {editorMode === 'flow' ? (
            <>
          <div className="flow-editor-tabs" role="tablist" aria-label="选择管道">
            <Segmented block options={flowOptions} value={activeFlowKey} onChange={(value) => selectFlowKey(value as FlowKey)} />
          </div>
          <p>
            {activeFlowMeta.description}。x 左右，y 高低，z 前后；滑块会实时刷新当前管线。
          </p>
          <div className="flow-editor-actions">
            <Button size="small" type="primary" onClick={addFlowPoint}>
              新增节点
            </Button>
            <span>至少保留 2 个节点</span>
          </div>
          <div className="flow-editor-points">
            {activeFlowPoints.map((point, index) => (
              <section className="flow-editor-point" key={`${activeFlowKey}-${index}`}>
                <div className="flow-editor-point-title">
                  <h3>
                    {index} · {getPointLabel(activeFlowKey, index)}
                  </h3>
                  <Button danger disabled={activeFlowPoints.length <= 2} size="small" onClick={() => removeFlowPoint(index)}>
                    删除
                  </Button>
                </div>
                {(['x', 'y', 'z'] as const).map((axis) => (
                  <label className="flow-editor-axis" key={axis}>
                    <span>{axis}</span>
                    <Slider
                      max={axis === 'y' ? 2.8 : 1.6}
                      min={axis === 'y' ? -1 : -1.6}
                      onChange={(value) => updateFlowPoint(activeFlowKey, index, axis, value)}
                      step={0.01}
                      value={point[axis]}
                    />
                    <InputNumber
                      max={axis === 'y' ? 2.8 : 1.6}
                      min={axis === 'y' ? -1 : -1.6}
                      onChange={(value) => updateFlowPoint(activeFlowKey, index, axis, Number(value ?? 0))}
                      step={0.01}
                      value={point[axis]}
                    />
                  </label>
                ))}
              </section>
            ))}
          </div>
          <div className="flow-editor-code">
            <div>
              <span>复制到 {activeFlowKey} 的 createPolylineCurve 里</span>
              <Button size="small" onClick={() => void copyActiveFlowPointCode()}>
                {copyState === 'copied' ? '已复制' : '复制'}
              </Button>
            </div>
            <textarea readOnly value={activePointCode} />
          </div>
            </>
          ) : (
            <>
              <div className="flow-editor-tabs" role="tablist" aria-label="选择标签">
                <Segmented block options={labelOptions} value={activeLabelKey} onChange={(value) => selectLabelKey(value as ModelLabelKey)} />
              </div>
              <p>
                {activeLabelMeta.oldOffset}。调试页会同时显示全部标签；正式驾驶舱按旧版顺序轮播，一次显示一个。x 左右，y 高低，z 前后。
              </p>
              <section className="flow-editor-point">
                <div className="flow-editor-point-title">
                  <h3>{activeLabelMeta.label}</h3>
                </div>
                {(['x', 'y', 'z'] as const).map((axis) => (
                  <label className="flow-editor-axis" key={axis}>
                    <span>{axis}</span>
                    <Slider
                      max={axis === 'y' ? 2.8 : 1.6}
                      min={axis === 'y' ? -1 : -1.6}
                      onChange={(value) => updateLabelPoint(activeLabelKey, axis, value)}
                      step={0.01}
                      value={activeLabelPoint[axis]}
                    />
                    <InputNumber
                      max={axis === 'y' ? 2.8 : 1.6}
                      min={axis === 'y' ? -1 : -1.6}
                      onChange={(value) => updateLabelPoint(activeLabelKey, axis, Number(value ?? 0))}
                      step={0.01}
                      value={activeLabelPoint[axis]}
                    />
                  </label>
                ))}
              </section>
              <div className="flow-editor-code">
                <div>
                  <span>复制到 DEFAULT_MODEL_LABEL_POINTS</span>
                  <Button size="small" onClick={() => void copyActiveLabelPointCode()}>
                    {copyState === 'copied' ? '已复制' : '复制'}
                  </Button>
                </div>
                <textarea readOnly value={activeLabelPointCode} />
              </div>
            </>
          )}
        </aside>
      ) : null}
    </div>
  )
}
