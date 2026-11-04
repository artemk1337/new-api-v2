import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'

import {
  getSupplierTariffs,
  publishSupplierTariffs,
  supplierQueryKeys,
  syncSupplierTariffs,
} from './api'
import type {
  SupplierModelTariff,
  SupplierPricingProfile,
  SupplierTariffs,
} from './types'

interface TariffsDialogProps {
  profile: SupplierPricingProfile
  onClose: () => void
}

export function TariffsDialog(props: TariffsDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<string | null>(null)
  const [validationError, setValidationError] = useState(false)
  const query = useQuery({
    queryKey: supplierQueryKeys.tariffs(props.profile.id),
    queryFn: () => getSupplierTariffs(props.profile.id),
  })
  const refresh = () => {
    setDraft(null)
    void queryClient.invalidateQueries({ queryKey: supplierQueryKeys.all })
  }
  const publish = useMutation({
    mutationFn: (value: {
      expected: SupplierTariffs
      tariffs: SupplierModelTariff[]
    }) =>
      publishSupplierTariffs(props.profile.id, value.expected, value.tariffs),
    onSuccess: () => {
      refresh()
      toast.success(t('Prices published'))
    },
  })
  const sync = useMutation({
    mutationFn: (expected: SupplierTariffs) =>
      syncSupplierTariffs(props.profile.id, expected),
    onSuccess: (data) => {
      refresh()
      toast.success(
        t(
          data.pending_confirmation
            ? 'Prices await a second matching fetch'
            : 'Prices published'
        )
      )
    },
  })
  const submit = () => {
    if (!query.data) return
    try {
      const value: unknown = JSON.parse(
        draft ?? JSON.stringify(query.data?.tariffs ?? [])
      )
      if (
        !Array.isArray(value) ||
        value.some(
          (item) =>
            !item ||
            typeof item !== 'object' ||
            typeof item.model_name !== 'string'
        )
      ) {
        setValidationError(true)
        return
      }
      setValidationError(false)
      publish.mutate({
        expected: query.data,
        tariffs: value as SupplierModelTariff[],
      })
    } catch {
      setValidationError(true)
    }
  }
  const pending = publish.isPending || sync.isPending

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={`${t('Profile prices')}: ${props.profile.name}`}
      description={t(
        'Complete procurement tariffs in real USD. Publishing replaces this profile catalog atomically.'
      )}
      contentHeight='auto'
      footer={
        <>
          <Button
            variant='outline'
            disabled={
              !query.data ||
              !props.profile.base_url ||
              pending ||
              draft !== null
            }
            onClick={() => {
              if (query.data) sync.mutate(query.data)
            }}
          >
            {t('Sync prices')}
          </Button>
          <Button
            disabled={!query.data || draft === null || pending}
            onClick={submit}
          >
            {t('Publish prices')}
          </Button>
        </>
      }
    >
      <div className='space-y-3'>
        <p className='text-muted-foreground text-sm'>
          {t('Revision')}: {query.data?.revision ?? '—'} ·{' '}
          {t(
            'Token prices are USD per million tokens; fixed prices use the declared unit.'
          )}
        </p>
        <Textarea
          aria-label={t('Profile prices')}
          className='min-h-80 font-mono text-xs'
          value={draft ?? JSON.stringify(query.data?.tariffs ?? [], null, 2)}
          onChange={(event) => setDraft(event.target.value)}
          disabled={!query.data || pending}
          spellCheck={false}
        />
        <details className='text-muted-foreground text-xs'>
          <summary className='cursor-pointer'>{t('Example')}</summary>
          <pre className='bg-muted mt-2 overflow-auto rounded-lg p-3'>
            {JSON.stringify(
              [
                {
                  model_name: 'example-model',
                  billing_mode: 'token',
                  input_usd: 1,
                  output_usd: 3,
                  cache_read_usd: 0.1,
                  cache_write_usd: 1.25,
                  cache_write_1h_usd: 2,
                  price_unit: 'million_tokens',
                },
              ],
              null,
              2
            )}
          </pre>
        </details>
        {validationError && (
          <p role='alert' className='text-destructive text-sm'>
            {t('Enter a valid tariff JSON array')}
          </p>
        )}
        {query.isError && (
          <p role='alert' className='text-destructive text-sm'>
            {t('Failed to load prices')}
          </p>
        )}
      </div>
    </Dialog>
  )
}
