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
import { motion, useInView, useReducedMotion } from 'motion/react'
import { useId, useRef } from 'react'

const strands = Array.from({ length: 36 }, (_, index) => ({
  id: index,
  path: `M -100 ${285 + index * 3} C 240 ${365 - index * 4}, 305 ${30 + index * 4}, 610 ${135 + index * 2} S 920 ${350 - index * 4}, 1300 ${35 + index * 3}`,
}))

export function HeroStudio() {
  const reducedMotion = useReducedMotion()
  const scene = useRef<HTMLDivElement>(null)
  const visible = useInView(scene)
  const gradient = useId()

  return (
    <motion.div
      ref={scene}
      className='studio-flow'
      data-moving={!reducedMotion && visible}
      initial={
        reducedMotion ? false : { opacity: 0, y: 24, filter: 'blur(12px)' }
      }
      animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
      transition={{
        duration: reducedMotion ? 0 : 1.4,
        delay: reducedMotion ? 0 : 0.15,
      }}
    >
      <div className='studio-flow-halo' aria-hidden='true' />
      <svg
        className='studio-flow-art'
        viewBox='0 0 1200 420'
        fill='none'
        aria-hidden='true'
      >
        <defs>
          <linearGradient
            id={gradient}
            x1='80'
            y1='280'
            x2='1100'
            y2='110'
            gradientUnits='userSpaceOnUse'
          >
            <stop stopColor='#77b9f0' stopOpacity='0' />
            <stop offset='.2' stopColor='#78a4ec' />
            <stop offset='.45' stopColor='#a991ee' />
            <stop offset='.68' stopColor='#7961ce' />
            <stop offset='.86' stopColor='#899fe6' />
            <stop offset='1' stopColor='#8cc9eb' stopOpacity='0' />
          </linearGradient>
          <linearGradient id={`${gradient}-light`}>
            <stop stopColor='#c2dcff' stopOpacity='0' />
            <stop offset='.5' stopColor='#e2d6ff' />
            <stop offset='1' stopColor='#bba2f3' stopOpacity='0' />
          </linearGradient>
        </defs>
        <g className='studio-flow-ribbon studio-flow-ribbon-back'>
          {strands.map((strand) => (
            <path
              key={strand.id}
              d={strand.path}
              stroke={`url(#${gradient})`}
              strokeWidth='1'
              opacity='.3'
            />
          ))}
        </g>
        <g className='studio-flow-ribbon studio-flow-ribbon-front'>
          {strands.map((strand) => (
            <path
              key={strand.id}
              d={strand.path}
              stroke={`url(#${gradient})`}
              strokeWidth='1.3'
              opacity={0.35 + strand.id / 80}
            />
          ))}
          {[8, 18, 28].map((index) => (
            <path
              className='studio-flow-trace'
              key={index}
              d={strands[index].path}
              stroke={`url(#${gradient}-light)`}
              strokeWidth='2'
              pathLength='1'
              style={{ animationDelay: `${-index / 4}s` }}
            />
          ))}
        </g>
      </svg>
    </motion.div>
  )
}
