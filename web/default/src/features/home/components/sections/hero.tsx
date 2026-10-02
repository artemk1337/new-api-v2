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
import { Link } from '@tanstack/react-router'
import { ArrowUpRight } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { Trans, useTranslation } from 'react-i18next'

import { HeroStudio } from '../hero-studio'

export function Hero(props: { isAuthenticated?: boolean }) {
  const { t } = useTranslation()
  const reducedMotion = useReducedMotion()

  return (
    <section className='studio-hero'>
      <div className='studio-intro'>
        <motion.h1
          initial={
            reducedMotion ? false : { opacity: 0, y: 18, filter: 'blur(8px)' }
          }
          animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
          transition={{ duration: reducedMotion ? 0 : 0.9 }}
        >
          <Trans
            i18nKey='Many models - <em>one API.</em>'
            components={{ em: <em /> }}
          />
        </motion.h1>
        <motion.div
          className='studio-actions'
          initial={reducedMotion ? false : { opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{
            duration: reducedMotion ? 0 : 0.7,
            delay: reducedMotion ? 0 : 0.2,
          }}
        >
          <Link
            className='studio-primary'
            to={props.isAuthenticated ? '/dashboard' : '/sign-up'}
          >
            {t('Start building')}
            <ArrowUpRight size={17} aria-hidden='true' />
          </Link>
          <Link className='studio-text-link' to='/pricing'>
            {t('Explore models')}
            <ArrowUpRight size={16} aria-hidden='true' />
          </Link>
        </motion.div>
      </div>
      <HeroStudio />
    </section>
  )
}
