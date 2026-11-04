import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatQuota } from '@/lib/format'

import { getKeyLogs, getKeySession, logoutKey } from './api'
import { KeyLoginForm } from './key-login-form'

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
  const logs = useQuery({
    queryKey: ['key-session', 'logs'],
    queryFn: getKeyLogs,
    enabled: !!session.data,
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

  if (session.isPending || session.isFetching) {
    return <main className='p-8'>{t('Loading')}</main>
  }

  if (!session.data) {
    return (
      <main className='flex min-h-svh items-center justify-center px-4'>
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
    <main className='mx-auto max-w-5xl space-y-6 px-4 py-8'>
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
      <Card>
        <CardHeader>
          <CardTitle>{t('Usage logs')}</CardTitle>
        </CardHeader>
        <CardContent className='overflow-x-auto'>
          {logs.isError && <p>{t('Request failed')}</p>}
          {logs.isPending && <p>{t('Loading')}</p>}
          {logs.isSuccess && logs.data.length === 0 && (
            <p className='text-muted-foreground'>{t('No data')}</p>
          )}
          {logs.isSuccess && logs.data.length > 0 && (
            <table className='w-full text-left text-sm'>
              <thead>
                <tr className='border-b'>
                  <th className='p-2'>{t('Time')}</th>
                  <th className='p-2'>{t('Model')}</th>
                  <th className='p-2'>{t('Input tokens')}</th>
                  <th className='p-2'>{t('Output tokens')}</th>
                  <th className='p-2'>{t('Usage')}</th>
                </tr>
              </thead>
              <tbody>
                {logs.data.map((log) => (
                  <tr key={log.id} className='border-b last:border-0'>
                    <td className='p-2 whitespace-nowrap'>
                      {new Date(log.created_at * 1000).toLocaleString()}
                    </td>
                    <td className='p-2'>{log.model_name}</td>
                    <td className='p-2'>
                      {log.prompt_tokens.toLocaleString()}
                    </td>
                    <td className='p-2'>
                      {log.completion_tokens.toLocaleString()}
                    </td>
                    <td className='p-2 whitespace-nowrap'>
                      {formatQuota(log.quota)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </CardContent>
      </Card>
    </main>
  )
}
