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
import { Gift, Link2, ShieldCheck, UserPlus, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

import { getReferralProgram, type ReferralProgram } from './api'

export function ReferralProgramPage() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['referral-program'],
    queryFn: getReferralProgram,
  })
  const program = query.data?.data

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='mx-auto w-full max-w-5xl space-y-8 px-4 py-14 sm:px-6 sm:py-20'>
        <section className='mx-auto max-w-3xl space-y-4 text-center'>
          <div className='bg-primary/10 text-primary mx-auto flex size-12 items-center justify-center rounded-2xl'>
            <Gift className='size-6' />
          </div>
          <h1 className='text-3xl font-semibold tracking-tight sm:text-4xl'>
            {t('Referral Program')}
          </h1>
          <p className='text-muted-foreground text-base sm:text-lg'>
            {t(
              'Invite friends, earn rewards from their top-ups, and give new users increased cashback.'
            )}
          </p>
        </section>

        <ReferralProgramContent
          isLoading={query.isLoading}
          program={program}
        />
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
          label={t('Additional referral cashback up to')}
          value={`+${program.referral_cashback_bonus_percent ?? 0}%`}
        />
        <ProgramValue
          label={t('Inviter reward from top-ups')}
          value={`${program.referral_deposit_percent}%`}
        />
        <ProgramValue
          label={t('Required top-up to share')}
          value={`$${program.required_topup_usd}`}
        />
      </div>

      <section className='grid gap-4 md:grid-cols-2'>
        <RuleCard
          icon={UserPlus}
          title={t('Register with a referral code')}
          description={t(
            'Follow an invitation link or enter the code during account creation. Existing accounts cannot add a referral code later.'
          )}
        />
        <RuleCard
          icon={WalletCards}
          title={t('Receive increased cashback')}
          description={t(
            'Referral cashback uses the cashback rate configured for each top-up tier.'
          )}
        />
        <RuleCard
          icon={ShieldCheck}
          title={t('Qualify before sharing')}
          description={t(
            'To activate your referral link, first complete successful top-ups totaling at least ${{amount}}.',
            { amount: program.required_topup_usd }
          )}
        />
        <RuleCard
          icon={Link2}
          title={t('Invite and earn')}
          description={t(
            'After activation, share your link and receive {{percent}}% from successful wallet top-ups made by your referrals.',
            { percent: program.referral_deposit_percent }
          )}
        />
      </section>

    </>
  )
}

function ProgramValue(props: { label: string; value: string }) {
  return (
    <Card className='text-center'>
      <CardHeader>
        <CardTitle className='text-2xl'>{props.value}</CardTitle>
        <CardDescription>{props.label}</CardDescription>
      </CardHeader>
    </Card>
  )
}

function RuleCard(props: {
  icon: typeof Gift
  title: string
  description: string
}) {
  const Icon = props.icon
  return (
    <Card>
      <CardHeader>
        <div className='bg-muted mb-2 flex size-9 items-center justify-center rounded-lg'>
          <Icon className='size-4' />
        </div>
        <CardTitle>{props.title}</CardTitle>
        <CardDescription>{props.description}</CardDescription>
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
        {Array.from({ length: 4 }, (_, index) => (
          <Skeleton key={index} className='h-36 rounded-xl' />
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
