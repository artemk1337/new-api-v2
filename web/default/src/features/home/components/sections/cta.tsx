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
import { useTranslation } from 'react-i18next'

interface CTAProps {
  isAuthenticated: boolean
}

export function CTA(props: CTAProps) {
  const { t } = useTranslation()

  return (
    <section
      className='studio-connect studio-cta'
      aria-labelledby='home-cta-title'
    >
      <h2 id='home-cta-title'>{t('Ready to get started?')}</h2>
      <div>
        <Link
          className='studio-primary'
          to={props.isAuthenticated ? '/keys' : '/sign-up'}
        >
          {t('Create an API key')}
          <ArrowUpRight size={16} aria-hidden='true' />
        </Link>
      </div>
    </section>
  )
}
