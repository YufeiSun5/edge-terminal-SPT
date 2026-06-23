import { useEffect, useRef } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'

const COCKPIT_MODEL_PATH = '/models/cockpit/new-shaded.glb'

export function CockpitModelStage() {
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
