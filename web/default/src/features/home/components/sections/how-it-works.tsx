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
import { useTranslation } from 'react-i18next'

export function HowItWorks() {
  const { t } = useTranslation()
  const reducedMotion = useReducedMotion()
  const steps = [
    { title: t('Create an API key'), to: '/keys' },
    { title: t('Choose your model'), to: '/pricing' },
    { title: t('Connect your app'), to: '/dashboard' },
  ] as const
  return (
    <section className='studio-connect'>
      <h2>{t('Get connected')}</h2>
      <ol className='studio-steps'>
        {steps.map((step, index) => (
          <motion.li
            key={step.title}
            className='studio-step'
            initial={reducedMotion ? false : { opacity: 0, x: 20 }}
            whileInView={{ opacity: 1, x: 0 }}
            viewport={{ once: true, amount: 0.5 }}
            transition={{
              duration: reducedMotion ? 0 : 0.45,
              delay: reducedMotion ? 0 : index * 0.1,
            }}
          >
            <Link className='studio-step-link' to={step.to}>
              <span className='studio-step-number'>0{index + 1}</span>
              {step.title}
              <ArrowUpRight size={18} aria-hidden='true' />
            </Link>
          </motion.li>
        ))}
      </ol>
    </section>
  )
}
