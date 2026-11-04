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
import type { LucideIcon } from 'lucide-react'

import { Card } from '@/components/ui/card'

type ReferralStep = {
  title: string
  description: string
}

export function ReferralPathCard(props: {
  icon: LucideIcon
  title: string
  steps: ReferralStep[]
}) {
  const Icon = props.icon

  return (
    <Card className='gap-0 p-5 sm:p-6'>
      <div className='flex items-center gap-3 border-b pb-5'>
        <div className='bg-muted flex size-9 shrink-0 items-center justify-center rounded-lg'>
          <Icon className='size-4.5' aria-hidden='true' />
        </div>
        <h2 className='text-lg font-semibold tracking-tight'>{props.title}</h2>
      </div>
      <ol className='space-y-5 pt-5'>
        {props.steps.map((step, index) => (
          <li key={step.title} className='flex items-start gap-3'>
            <span
              className='text-muted-foreground mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full border text-xs'
              aria-hidden='true'
            >
              {index + 1}
            </span>
            <div>
              <h3 className='text-sm leading-normal font-medium'>
                {step.title}
              </h3>
              <p className='text-muted-foreground mt-1 text-sm leading-relaxed'>
                {step.description}
              </p>
            </div>
          </li>
        ))}
      </ol>
    </Card>
  )
}
