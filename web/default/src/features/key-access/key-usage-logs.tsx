import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatQuota } from '@/lib/format'

import { getKeyLogs } from './api'

export function KeyUsageLogs() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const logs = useQuery({
    queryKey: ['key-session', 'logs', page],
    queryFn: () => getKeyLogs(page),
    retry: false,
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Usage logs')}</CardTitle>
      </CardHeader>
      <CardContent className='overflow-x-auto'>
        {logs.isError && <p>{t('Request failed')}</p>}
        {logs.isPending && <p>{t('Loading')}</p>}
        {logs.isSuccess && logs.data.items.length === 0 && (
          <p className='text-muted-foreground'>{t('No data')}</p>
        )}
        {logs.isSuccess && logs.data.items.length > 0 && (
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
              {logs.data.items.map((log) => (
                <tr key={log.id} className='border-b last:border-0'>
                  <td className='p-2 whitespace-nowrap'>
                    {new Date(log.created_at * 1000).toLocaleString()}
                  </td>
                  <td className='p-2'>{log.model_name}</td>
                  <td className='p-2'>{log.prompt_tokens.toLocaleString()}</td>
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
      {logs.isSuccess && logs.data.total > 0 && (
        <nav
          aria-label={t('Pagination')}
          className='flex flex-wrap items-center justify-between gap-3 border-t px-6 pt-4'
        >
          <p className='text-muted-foreground text-sm'>
            {t('Page {{page}} of {{total}}', {
              page: logs.data.page,
              total: Math.max(
                1,
                Math.ceil(logs.data.total / logs.data.page_size)
              ),
            })}
          </p>
          <div className='flex gap-2'>
            <Button
              variant='outline'
              disabled={page <= 1 || logs.isFetching}
              onClick={() => setPage(page - 1)}
            >
              {t('Previous page')}
            </Button>
            <Button
              variant='outline'
              disabled={
                page * logs.data.page_size >= logs.data.total || logs.isFetching
              }
              onClick={() => setPage(page + 1)}
            >
              {t('Next page')}
            </Button>
          </div>
        </nav>
      )}
      {logs.isError && page > 1 && (
        <Button
          variant='outline'
          className='mx-6'
          onClick={() => setPage(page - 1)}
        >
          {t('Previous page')}
        </Button>
      )}
    </Card>
  )
}
