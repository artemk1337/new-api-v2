import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'

import {
  getUserTokens,
  setUserTokenStatus,
  type UserToken,
} from '../../token-api'

interface Props {
  user: { id: number; username: string }
  onOpenChange: (open: boolean) => void
}

const statusLabels: Record<number, string> = {
  1: 'Enabled',
  2: 'Disabled',
  3: 'Expired',
  4: 'Exhausted',
  5: 'Disabled by administrator',
}

export function UserTokensDialog(props: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [target, setTarget] = useState<UserToken | null>(null)
  const queryKey = ['user-tokens', props.user.id]
  const query = useQuery({
    queryKey: [...queryKey, page],
    queryFn: () => getUserTokens(props.user.id, page),
  })
  const mutation = useMutation({
    mutationFn: (token: UserToken) =>
      setUserTokenStatus(props.user.id, token.id, token.status === 5 ? 1 : 2),
    onSuccess: () => {
      setTarget(null)
      toast.success(t('Updated successfully'))
      void queryClient.invalidateQueries({ queryKey })
      void queryClient.invalidateQueries({ queryKey: ['keys'] })
    },
  })

  return (
    <>
      <Dialog
        open
        onOpenChange={props.onOpenChange}
        title={t('API Keys')}
        description={props.user.username}
        contentClassName='sm:max-w-xl'
        bodyClassName='space-y-3'
        footer={
          <div className='flex w-full items-center justify-between gap-2'>
            <Button
              variant='outline'
              size='sm'
              aria-label={t('Previous page')}
              disabled={page === 1 || query.isFetching}
              onClick={() => setPage(page - 1)}
            >
              <ChevronLeft aria-hidden='true' />
              <span className='hidden sm:inline'>{t('Previous page')}</span>
            </Button>
            <span className='text-muted-foreground text-sm whitespace-nowrap'>
              {page} / {Math.max(1, Math.ceil((query.data?.total || 0) / 10))}
            </span>
            <Button
              variant='outline'
              size='sm'
              aria-label={t('Next page')}
              disabled={
                page * 10 >= (query.data?.total || 0) || query.isFetching
              }
              onClick={() => setPage(page + 1)}
            >
              <span className='hidden sm:inline'>{t('Next page')}</span>
              <ChevronRight aria-hidden='true' />
            </Button>
          </div>
        }
      >
        <p className='text-muted-foreground text-sm'>
          {t(
            'Admin-disabled keys reject new requests before usage logging. Only an administrator can re-enable them.'
          )}
        </p>
        {query.isPending && <p role='status'>{t('Loading...')}</p>}
        {query.isError && (
          <div role='alert' className='space-y-2'>
            <p>{t('Failed to load API keys')}</p>
            <Button variant='outline' onClick={() => void query.refetch()}>
              {t('Retry')}
            </Button>
          </div>
        )}
        {query.data?.items.length === 0 && (
          <p className='text-muted-foreground py-6 text-center'>
            {t('No API Keys Found')}
          </p>
        )}
        {query.data?.items.map((token) => (
          <div
            key={token.id}
            className='flex flex-col items-stretch gap-3 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between'
          >
            <div className='min-w-0 space-y-1'>
              <p className='text-sm font-medium break-all'>
                {token.name || `#${token.id}`}
              </p>
              <p className='text-muted-foreground text-xs break-all'>
                #{token.id}
                {token.group && ` · ${token.group}`}
              </p>
              <StatusBadge
                label={t(statusLabels[token.status] || 'Unknown')}
                variant={token.status === 1 ? 'success' : 'neutral'}
                copyable={false}
                size='sm'
              />
            </div>
            <Button
              variant='outline'
              size='sm'
              className='shrink-0 self-end sm:self-auto'
              disabled={mutation.isPending}
              onClick={() => setTarget(token)}
            >
              {token.status === 5 ? t('Enable') : t('Disable')}
            </Button>
          </div>
        ))}
      </Dialog>
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(open) => !open && !mutation.isPending && setTarget(null)}
        title={
          target?.status === 5 ? t('Enable API key') : t('Disable API key')
        }
        desc={t('Change access for API key {{name}}?', {
          name: target?.name || `#${target?.id}`,
        })}
        confirmText={target?.status === 5 ? t('Enable') : t('Disable')}
        destructive={target?.status !== 5}
        isLoading={mutation.isPending}
        handleConfirm={() => target && mutation.mutate(target)}
      />
    </>
  )
}
