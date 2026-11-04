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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ArrowUpRight, Gift, Link2, UserPlus } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

import { getReferralProgram, type ReferralProgram } from './api'
import { ReferralPathCard } from './referral-path-card'

export function ReferralProgramPage() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['referral-program'],
    queryFn: getReferralProgram,
  })
  const program = query.data?.data

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='mx-auto w-full max-w-5xl space-y-8 px-4 py-14 sm:px-6 sm:py-24'>
        <section className='mx-auto max-w-3xl space-y-4 text-center'>
          <div className='bg-primary/10 text-primary mx-auto flex size-12 items-center justify-center rounded-2xl'>
            <Gift className='size-6' />
          </div>
          <h1 className='text-3xl font-semibold tracking-tight sm:text-4xl'>
            {t('Referral Program')}
          </h1>
          {program && (
            <p className='text-muted-foreground text-base sm:text-lg'>
              {t(
                'Earn {{percent}}% from friends’ top-ups and give them up to +{{bonus}}% cashback.',
                {
                  percent: program.referral_deposit_percent,
                  bonus: program.referral_cashback_bonus_percent ?? 0,
                }
              )}
            </p>
          )}
        </section>

        <ReferralProgramContent isLoading={query.isLoading} program={program} />
      </PageTransition>
    </PublicLayout>
  )
}

function ReferralProgramContent(props: {
  isLoading: boolean
  program?: ReferralProgram
}) {
  const { t } = useTranslation()
  if (props.isLoading) return <ReferralProgramSkeleton />
  if (!props.program) {
    return (
      <Card>
        <CardContent className='text-muted-foreground py-8 text-center'>
          {t('Referral program information is temporarily unavailable.')}
        </CardContent>
      </Card>
    )
  }

  const program = props.program
  return (
    <>
      <div className='grid gap-4 sm:grid-cols-3'>
        <ProgramValue
          label={t('Additional cashback for your friend')}
          value={t('Up to +{{percent}}%', {
            percent: program.referral_cashback_bonus_percent ?? 0,
          })}
        />
        <ProgramValue
          label={t('Your reward from friends’ top-ups')}
          value={`${program.referral_deposit_percent}%`}
        />
        <ProgramValue
          label={t('Top-ups to activate your link')}
          value={`$${program.required_topup_usd}`}
        />
      </div>

      <div className='flex flex-col items-start justify-between gap-6 border-y py-5 sm:flex-row sm:items-center'>
        <div>
          <p className='font-medium'>{t('Invite friends to VibeCode API')}</p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t(
              'Your invitation link is available in your dashboard after top-ups totaling ${{amount}}.',
              { amount: program.required_topup_usd }
            )}
          </p>
        </div>
        <Button
          className='h-10 gap-3 rounded-full px-4'
          render={<Link to='/dashboard' />}
        >
          {t('Go to dashboard')}
          <ArrowUpRight aria-hidden='true' />
        </Button>
      </div>

      <section
        aria-label={t('How to participate')}
        className='grid gap-4 md:grid-cols-2'
      >
        <ReferralPathCard
          icon={UserPlus}
          title={t('You were invited')}
          steps={[
            {
              title: t('Sign up using your friend’s link'),
              description: t(
                'Or enter their referral code when creating your account.'
              ),
            },
            {
              title: t('Receive increased cashback'),
              description: t(
                'Top up your balance to receive a referral cashback bonus. The bonus depends on your top-up tier.'
              ),
            },
          ]}
        />
        <ReferralPathCard
          icon={Link2}
          title={t('You invite friends')}
          steps={[
            {
              title: t('Activate your link'),
              description: t(
                'Top up your balance by a total of ${{amount}} to unlock your invitation link in the dashboard.',
                { amount: program.required_topup_usd }
              ),
            },
            {
              title: t('Share your link with friends'),
              description: t(
                'Earn {{percent}}% from successful wallet top-ups by friends who signed up using your invitation.',
                { percent: program.referral_deposit_percent }
              ),
            },
          ]}
        />
      </section>
    </>
  )
}

function ProgramValue(props: { label: string; value: string }) {
  return (
    <Card className='min-h-25 justify-center text-center'>
      <CardHeader>
        <CardTitle className='text-2xl'>{props.value}</CardTitle>
        <CardDescription>{props.label}</CardDescription>
      </CardHeader>
    </Card>
  )
}

function ReferralProgramSkeleton() {
  return (
    <div className='space-y-4'>
      <div className='grid gap-4 sm:grid-cols-3'>
        {Array.from({ length: 3 }, (_, index) => (
          <Skeleton key={index} className='h-28 rounded-xl' />
        ))}
      </div>
      <div className='grid gap-4 md:grid-cols-2'>
        {Array.from({ length: 2 }, (_, index) => (
          <Skeleton key={index} className='h-72 rounded-xl' />
        ))}
      </div>
    </div>
  )
}
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
