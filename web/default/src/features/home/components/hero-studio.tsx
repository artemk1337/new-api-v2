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
import { useRef } from 'react'

import { HeroFlowArt } from './hero-flow-art'

export function HeroStudio() {
  const reducedMotion = useReducedMotion()
  const scene = useRef<HTMLDivElement>(null)
  const visible = useInView(scene)

  return (
    <motion.div
      ref={scene}
      className='studio-flow studio-provider-flow'
      data-moving={!reducedMotion && visible}
      initial={reducedMotion ? false : { opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: reducedMotion ? 0 : 0.4, ease: 'easeOut' }}
    >
      <HeroFlowArt mobile={false} />
      <HeroFlowArt mobile />
    </motion.div>
  )
}
