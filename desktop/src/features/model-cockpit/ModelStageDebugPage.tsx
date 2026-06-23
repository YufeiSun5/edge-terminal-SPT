import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Box, Rotate3D, Waves } from 'lucide-react'
import { CockpitModelStage } from '@/features/model-cockpit/components/CockpitModelStage'
import './model-stage-debug.css'

const debugItems = [
  { key: 'model', icon: Box },
  { key: 'orbit', icon: Rotate3D },
  { key: 'fluid', icon: Waves },
] as const

export function ModelStageDebugPage() {
  const { t } = useTranslation()

  useEffect(() => {
    const resetScroll = () => {
      window.scrollTo({ top: 0, left: 0 })
      document.querySelector<HTMLElement>('.workbench-content')?.scrollTo({ top: 0, left: 0 })
    }
    resetScroll()
    const frame = window.requestAnimationFrame(resetScroll)
    return () => window.cancelAnimationFrame(frame)
  }, [])

  return (
    <div className="model-stage-debug-page">
      <header className="model-stage-debug-header">
        <div>
          <span>{t('modelCockpit.debug.eyebrow')}</span>
          <h2>{t('modelCockpit.debug.title')}</h2>
        </div>
      </header>

      <section className="model-stage-debug-shell" aria-label={t('modelCockpit.debug.stage')}>
        <div className="model-stage-debug-toolbar">
          {debugItems.map((item) => {
            const Icon = item.icon
            return (
              <div className="model-stage-debug-chip" key={item.key}>
                <Icon aria-hidden="true" size={16} />
                <span>{t(`modelCockpit.debug.items.${item.key}`)}</span>
              </div>
            )
          })}
        </div>
        <div className="model-stage-debug-canvas">
          <CockpitModelStage showFlowEditor />
        </div>
      </section>
    </div>
  )
}
