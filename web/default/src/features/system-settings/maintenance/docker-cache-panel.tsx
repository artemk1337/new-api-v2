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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { api } from '@/lib/api'

type DockerCacheStats = {
  total_size: number
  reclaimable_size: number
  record_count: number
}

type DockerCacheResponse<T> =
  | { success: true; data: T }
  | { success: false; message: string }

type Props = {
  onCleanup: () => void | Promise<void>
}

const queryKey = ['performance', 'docker-cache']
const endpoint = '/api/performance/docker_cache'

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), 4)
  return `${Number((bytes / 1024 ** unit).toFixed(2))} ${units[unit]}`
}

export function DockerCachePanel(props: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const stats = useQuery({
    queryKey,
    queryFn: async () => {
      const response = await api.get<DockerCacheResponse<DockerCacheStats>>(
        endpoint,
        { timeout: 180000, skipBusinessError: true, skipErrorHandler: true }
      )
      if (!response.data.success) throw new Error(response.data.message)
      return response.data.data
    },
    retry: false,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const cleanup = useMutation({
    mutationFn: async () => {
      const response = await api.delete<
        DockerCacheResponse<{ space_reclaimed: number }>
      >(endpoint, {
        timeout: 180000,
        skipBusinessError: true,
        skipErrorHandler: true,
      })
      if (!response.data.success) throw new Error(response.data.message)
      return response.data.data
    },
    onSuccess: (data) => {
      setConfirmOpen(false)
      toast.success(
        t('Docker build cache cleared. Freed {{size}}.', {
          size: formatBytes(data.space_reclaimed),
        })
      )
      void queryClient.invalidateQueries({ queryKey })
      void props.onCleanup()
    },
    onError: (error) => {
      toast.error(t('Cleanup failed'), { description: t(error.message) })
    },
  })

  return (
    <section className='space-y-4 rounded-lg border p-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='min-w-0 flex-1 basis-64'>
          <h4 className='text-sm font-medium'>{t('Docker build cache')}</h4>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t(
              'Storage used by Docker builds on the host, separate from the application cache.'
            )}
          </p>
        </div>
        <div className='flex flex-wrap gap-2'>
          <Button
            variant='outline'
            size='sm'
            disabled={stats.isFetching || cleanup.isPending}
            onClick={() => void stats.refetch()}
          >
            {stats.isFetching && <Spinner aria-label={t('Loading')} />}
            {t('Refresh Docker cache')}
          </Button>
          <AlertDialog
            open={confirmOpen}
            onOpenChange={(open) => {
              if (!cleanup.isPending) setConfirmOpen(open)
            }}
          >
            <AlertDialogTrigger
              render={<Button variant='outline' size='sm' />}
              disabled={
                !stats.isSuccess || stats.isFetching || cleanup.isPending
              }
            >
              {cleanup.isPending && <Spinner aria-label={t('Loading')} />}
              {cleanup.isPending
                ? t('Cleaning Docker cache...')
                : t('Clean Docker build cache')}
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  {t('Clean Docker build cache?')}
                </AlertDialogTitle>
                <AlertDialogDescription>
                  {t(
                    'This removes unused Docker build cache older than 24 hours. Containers, images, and volumes are preserved.'
                  )}
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={cleanup.isPending}>
                  {t('Cancel')}
                </AlertDialogCancel>
                <AlertDialogAction
                  disabled={cleanup.isPending}
                  onClick={() => cleanup.mutate()}
                >
                  {cleanup.isPending && <Spinner aria-label={t('Loading')} />}
                  {cleanup.isPending
                    ? t('Cleaning Docker cache...')
                    : t('Confirm Cleanup')}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      </div>
      {stats.isPending && (
        <p className='text-muted-foreground text-xs' role='status'>
          {t('Loading...')}
        </p>
      )}
      {stats.isError && (
        <Alert>
          <AlertDescription>
            <p>{t('Unable to load Docker build cache.')}</p>
            <p>{t(stats.error.message)}</p>
          </AlertDescription>
        </Alert>
      )}
      {stats.isSuccess && (
        <>
          <dl className='grid gap-3 text-xs sm:grid-cols-3'>
            <div>
              <dt className='text-muted-foreground'>
                {t('Total build cache')}
              </dt>
              <dd className='mt-1 text-sm font-medium tabular-nums'>
                {formatBytes(stats.data.total_size)}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>
                {t('Estimated reclaimable')}
              </dt>
              <dd className='mt-1 text-sm font-medium tabular-nums'>
                {formatBytes(stats.data.reclaimable_size)}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Cache records')}</dt>
              <dd className='mt-1 text-sm font-medium tabular-nums'>
                {stats.data.record_count}
              </dd>
            </div>
          </dl>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Estimate includes all unused cache. Cleanup only removes records unused for at least 24 hours; shared layers may reduce the space freed.'
            )}{' '}
            {t(
              'Separate Buildx builders, images, and volumes are not included.'
            )}
          </p>
        </>
      )}
    </section>
  )
}
