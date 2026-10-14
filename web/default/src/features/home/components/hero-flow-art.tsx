/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useTranslation } from 'react-i18next'

import { useSystemConfigStore } from '@/stores/system-config-store'

const providers = [
  'OpenAI',
  'Anthropic',
  'Google',
  'DeepSeek',
  'Qwen',
  'Meta',
  'xAI',
]

function createScene(mobile: boolean) {
  const width = mobile ? 380 : 900
  const height = mobile ? 340 : 400
  const source = mobile ? 52 : 90
  const hub = mobile ? 150 : 365
  const end = mobile ? 276 : 735
  const center = height / 2
  const gap = mobile ? 39 : 46
  const bend = mobile ? 33 : 70
  const incoming = Array.from({ length: 18 }, (_, index) => {
    const offset = (index - 8.5) * (mobile ? 0.35 : 0.65)
    return `M ${source + 10} ${center + offset} C ${source + bend} ${center + offset} ${hub - bend} ${center + offset} ${hub} ${center + offset}`
  })
  const branches = providers.map((name, index) => {
    const y = center + (index - 3) * gap
    const paths = Array.from({ length: 3 }, (_, strand) => {
      const offset = (strand - 1) * (mobile ? 1.1 : 2.1)
      return `M ${hub} ${center + offset} C ${hub + bend * 1.7} ${center + offset} ${end - bend * 1.4} ${y + offset} ${end} ${y + offset}`
    })
    return { name, y, paths }
  })
  return { width, height, source, hub, end, center, incoming, branches }
}

const desktop = createScene(false)
const mobile = createScene(true)

export function HeroFlowArt(props: { mobile: boolean }) {
  const { t } = useTranslation()
  const systemName = useSystemConfigStore(
    (state) => state.config.homePageFlowLabel?.trim() || state.config.systemName
  )
  const scene = props.mobile ? mobile : desktop
  const sourceWidth = props.mobile ? 54 : 64
  const hubWidth = props.mobile ? 70 : 98
  const capsuleWidth = props.mobile ? 77 : 98

  return (
    <svg
      className={
        props.mobile
          ? 'studio-network studio-network-mobile'
          : 'studio-network studio-network-desktop'
      }
      viewBox={`0 0 ${scene.width} ${scene.height}`}
      fill='none'
      role='img'
      aria-label={t(
        'Your application connects to AI providers through {{platform}}',
        { platform: systemName }
      )}
    >
      {scene.incoming.map((path, index) => (
        <g key={path}>
          <path
            className='studio-network-strand studio-network-input'
            d={path}
          />
          {index % 6 === 0 && (
            <path
              className='studio-network-pulse studio-network-input'
              d={path}
              pathLength={1}
              style={{ animationDelay: `${-index * 0.12}s` }}
            />
          )}
        </g>
      ))}
      {scene.branches.map((branch, index) => (
        <g key={branch.name}>
          {branch.paths.map((path) => (
            <path key={path} className='studio-network-strand' d={path} />
          ))}
          <path
            className='studio-network-pulse'
            d={branch.paths[1]}
            pathLength={1}
            style={{ animationDelay: `${-index * 0.65}s` }}
          />
          <circle
            className='studio-network-endpoint'
            cx={scene.end}
            cy={branch.y}
            r={3}
          />
          <rect
            className='studio-network-capsule'
            x={scene.end + 9}
            y={branch.y - 14}
            width={capsuleWidth}
            height={28}
            rx={14}
          />
          <text
            className='studio-network-name'
            x={scene.end + 9 + capsuleWidth / 2}
            y={branch.y + 5}
            textAnchor='middle'
          >
            {branch.name}
          </text>
        </g>
      ))}
      <rect
        className='studio-network-capsule'
        x={scene.source - sourceWidth / 2}
        y={scene.center - 20}
        width={sourceWidth}
        height={40}
        rx={10}
      />
      <path
        className='studio-network-browser-line'
        d={`M ${scene.source - sourceWidth / 2 + 1} ${scene.center - 7} H ${scene.source + sourceWidth / 2 - 1}`}
      />
      <circle
        className='studio-network-browser-dot'
        cx={scene.source - sourceWidth / 2 + 9}
        cy={scene.center - 13}
        r={1.5}
      />
      <text
        className='studio-network-code'
        x={scene.source}
        y={scene.center + 11}
        textAnchor='middle'
      >
        ···
      </text>
      <text
        className='studio-network-source'
        x={scene.source}
        y={scene.center + 43}
        textAnchor='middle'
      >
        {t('Application')}
      </text>
      <rect
        className='studio-network-hub'
        x={scene.hub - hubWidth / 2}
        y={scene.center - 21}
        width={hubWidth}
        height={42}
        rx={21}
      />
      <text
        className='studio-network-platform'
        x={scene.hub}
        y={scene.center + 5}
        textAnchor='middle'
        textLength={systemName.length > 12 ? hubWidth - 12 : undefined}
        lengthAdjust='spacingAndGlyphs'
      >
        {systemName}
      </text>
    </svg>
  )
}
