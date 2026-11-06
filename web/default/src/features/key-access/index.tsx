import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatQuota } from '@/lib/format'

import { getKeySession, logoutKey } from './api'
import { KeyLoginForm } from './key-login-form'
import { KeyUsageLogs } from './key-usage-logs'

export { KeyLoginForm } from './key-login-form'

export function KeyAccess() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [pending, setPending] = useState(false)
  const session = useQuery({
    queryKey: ['key-session'],
    queryFn: getKeySession,
    retry: false,
  })

  async function signOut() {
    setPending(true)
    try {
      await logoutKey()
      queryClient.removeQueries({ queryKey: ['key-session', 'logs'] })
      queryClient.setQueryData(['key-session'], null)
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setPending(false)
    }
  }

  if (session.isPending) {
    return <main className='px-4 pt-24'>{t('Loading')}</main>
  }

  if (!session.data) {
    return (
      <main className='flex min-h-svh items-center justify-center px-4 pt-20'>
        <Card className='w-full max-w-md'>
          <CardHeader>
            <CardTitle>{t('Sign in with API key')}</CardTitle>
          </CardHeader>
          <CardContent className='space-y-4'>
            <KeyLoginForm />
            <Link
              to='/sign-in'
              className='text-muted-foreground block text-center text-sm underline'
            >
              {t('Sign in')}
            </Link>
          </CardContent>
        </Card>
      </main>
    )
  }

  return (
    <main className='mx-auto max-w-5xl space-y-6 px-4 pt-24 pb-8'>
      <header className='flex flex-wrap items-center justify-between gap-4'>
        <div>
          <h1 className='text-2xl font-semibold'>{t('Usage')}</h1>
          <p className='text-muted-foreground'>{session.data.name}</p>
        </div>
        <div className='flex gap-2'>
          <Button
            variant='outline'
            onClick={() =>
              queryClient.invalidateQueries({ queryKey: ['key-session'] })
            }
          >
            {t('Refresh')}
          </Button>
          <Button variant='outline' onClick={signOut} disabled={pending}>
            {t('Sign out')}
          </Button>
        </div>
      </header>
      <div className='grid gap-4 sm:grid-cols-2'>
        <Card>
          <CardHeader>
            <CardTitle className='text-sm'>{t('Used')}</CardTitle>
          </CardHeader>
          <CardContent className='text-2xl font-semibold'>
            {formatQuota(session.data.used_quota)}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className='text-sm'>{t('Remaining quota')}</CardTitle>
          </CardHeader>
          <CardContent className='text-2xl font-semibold'>
            {session.data.unlimited_quota
              ? t('Unlimited')
              : formatQuota(session.data.remain_quota)}
          </CardContent>
        </Card>
      </div>
      <aside className='bg-primary/5 border-primary/15 flex flex-col gap-4 rounded-xl border p-5 sm:flex-row sm:items-center sm:justify-between'>
        <div className='space-y-1'>
          <p className='font-medium'>{t('Get more from the platform')}</p>
          <p className='text-muted-foreground text-sm'>
            {t('Register to access detailed statistics and advanced features.')}
          </p>
        </div>
        <Button className='shrink-0' render={<Link to='/sign-up' />}>
          {t('Sign up')}
        </Button>
      </aside>
      <KeyUsageLogs />
    </main>
  )
}
