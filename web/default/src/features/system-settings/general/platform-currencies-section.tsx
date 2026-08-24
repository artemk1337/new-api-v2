import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import { SettingsSection } from '../components/settings-section'
import {
  createPlatformCurrency,
  deletePlatformCurrency,
  getCurrencySyncConfig,
  getPlatformCurrencies,
  syncAllPlatformCurrencies,
  syncPlatformCurrency,
  updateCurrencySyncConfig,
  updatePlatformCurrency,
  type PlatformCurrency,
  type PlatformCurrencyPayload,
} from './platform-currencies-api'
import {
  getSupportedSyncProviders,
  getSyncIntervalLabel,
  getSyncProviderLabel,
} from './platform-currencies-sync'

type Draft = PlatformCurrencyPayload

const emptyDraft: Draft = {
  code: '',
  name: '',
  symbol: '',
  enabled: true,
  sync_enabled: false,
  sync_provider: 'cbr',
  manual_rate_to_usd: 1,
}

export function PlatformCurrenciesSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<PlatformCurrency | null>(null)
  const [draft, setDraft] = useState<Draft>(emptyDraft)
  const [syncInterval, setSyncInterval] = useState('day')
  const currenciesQuery = useQuery({
    queryKey: ['platform-currencies', 'admin'],
    queryFn: () => getPlatformCurrencies(true),
  })
  const currencies = useMemo(
    () => currenciesQuery.data?.data ?? [],
    [currenciesQuery.data?.data]
  )
  const syncConfigQuery = useQuery({
    queryKey: ['platform-currencies', 'sync-config'],
    queryFn: getCurrencySyncConfig,
  })
  const syncConfig = syncConfigQuery.data?.data
  const configuredSyncInterval = syncConfig?.update_interval ?? 'day'
  const allowedIntervals = syncConfig?.allowed_intervals ?? []
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['platform-currencies'] })
  const saveMutation = useMutation({
    mutationFn: () =>
      editing
        ? updatePlatformCurrency(editing.code, draft)
        : createPlatformCurrency(draft),
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Failed to save currency'))
        return
      }
      toast.success(t('Currency saved'))
      setDialogOpen(false)
      invalidate()
    },
    onError: (error) =>
      toast.error(
        error instanceof Error ? error.message : t('Failed to save currency')
      ),
  })
  const syncMutation = useMutation({
    mutationFn: syncPlatformCurrency,
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Failed to synchronize currency'))
        return
      }
      toast.success(t('Currency synchronized'))
      invalidate()
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to synchronize currency')
      ),
  })
  const updateSyncConfigMutation = useMutation({
    mutationFn: updateCurrencySyncConfig,
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        toast.error(response.message || t('Failed to save synchronization settings'))
        return
      }
      setSyncInterval(response.data.update_interval)
      queryClient.setQueryData(
        ['platform-currencies', 'sync-config'],
        response
      )
      toast.success(t('Synchronization settings saved'))
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to save synchronization settings')
      ),
  })
  const batchSyncMutation = useMutation({
    mutationFn: syncAllPlatformCurrencies,
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Failed to synchronize currencies'))
        return
      }
      toast.success(t('Currencies synchronized'))
      invalidate()
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to synchronize currencies')
      ),
  })
  const deleteMutation = useMutation({
    mutationFn: deletePlatformCurrency,
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Failed to delete currency'))
        return
      }
      toast.success(t('Currency deleted'))
      invalidate()
    },
    onError: (error) =>
      toast.error(
        error instanceof Error ? error.message : t('Failed to delete currency')
      ),
  })
  const sortedCurrencies = useMemo(
    () => [...currencies].sort((a, b) => a.code.localeCompare(b.code)),
    [currencies]
  )
  const synchronizedCurrencies = useMemo(
    () => currencies.filter((currency) => currency.sync_enabled),
    [currencies]
  )
  useEffect(() => {
    if (syncConfig) setSyncInterval(syncConfig.update_interval)
  }, [syncConfig])

  const openCreate = () => {
    setEditing(null)
    setDraft(emptyDraft)
    setDialogOpen(true)
  }
  const openEdit = (currency: PlatformCurrency) => {
    const supportedProviders = getSupportedSyncProviders(currency.code)
    setEditing(currency)
    setDraft({
      code: currency.code,
      name: currency.name,
      symbol: currency.symbol,
      enabled: currency.enabled,
      sync_enabled: currency.sync_enabled && supportedProviders.length > 0,
      sync_provider: supportedProviders.includes(currency.sync_provider)
        ? currency.sync_provider
        : (supportedProviders[0] ?? ''),
      manual_rate_to_usd:
        currency.manual_rate_to_usd || currency.rate_to_usd || 1,
    })
    setDialogOpen(true)
  }
  const setDraftField = <K extends keyof Draft>(key: K, value: Draft[K]) => {
    setDraft((current) => ({ ...current, [key]: value }))
  }
  const toggleDraftSync = (checked: boolean) => {
    if (!checked) {
      setDraftField('sync_enabled', false)
      return
    }
    const providers = getSupportedSyncProviders(draft.code)
    if (providers.length === 0) return
    setDraft((current) => ({
      ...current,
      sync_enabled: true,
      sync_provider: providers.includes(current.sync_provider)
        ? current.sync_provider
        : providers[0],
    }))
  }

  const renderCurrencyList = () => {
    if (currenciesQuery.isLoading) {
      return (
        <div className='text-muted-foreground rounded-lg border p-6 text-center text-sm'>
          {t('Loading...')}
        </div>
      )
    }
    if (sortedCurrencies.length === 0) {
      return (
        <div className='text-muted-foreground rounded-lg border border-dashed p-6 text-center text-sm'>
          {t('No currencies configured')}
        </div>
      )
    }
    return (
      <div className='overflow-x-auto rounded-lg border'>
        <table className='w-full text-sm'>
          <thead className='bg-muted/40 text-muted-foreground text-left'>
            <tr>
              <th className='px-3 py-2 font-medium'>{t('Currency')}</th>
              <th className='px-3 py-2 font-medium'>{t('Rate')}</th>
              <th className='px-3 py-2 font-medium'>{t('Source')}</th>
              <th className='px-3 py-2 font-medium'>{t('Status')}</th>
              <th className='px-3 py-2 text-right font-medium'>
                {t('Actions')}
              </th>
            </tr>
          </thead>
          <tbody className='divide-y'>
            {sortedCurrencies.map((currency) => (
              <tr key={currency.code}>
                <td className='px-3 py-3'>
                  <div className='font-medium'>
                    {currency.symbol} {currency.code}
                  </div>
                  <div className='text-muted-foreground text-xs'>
                    {currency.name}
                  </div>
                </td>
                <td className='px-3 py-3 font-mono text-xs'>
                  1 USD = {currency.rate_to_usd} {currency.code}
                </td>
                <td className='px-3 py-3'>
                  {currency.sync_enabled
                    ? t('Sync: {{provider}}', {
                        provider:
                          getSyncProviderLabel(currency.sync_provider),
                      })
                    : t('Manual')}
                </td>
                <td className='px-3 py-3'>
                  <span
                    className={
                      currency.enabled
                        ? 'text-emerald-600'
                        : 'text-muted-foreground'
                    }
                  >
                    {currency.enabled ? t('Enabled') : t('Disabled')}
                  </span>
                  {currency.last_sync_error && (
                    <div className='text-destructive mt-1 text-xs'>
                      {currency.last_sync_error}
                    </div>
                  )}
                </td>
                <td className='px-3 py-3'>
                  <div className='flex justify-end gap-1'>
                    {currency.sync_enabled && (
                      <Button
                        type='button'
                        size='icon'
                        variant='ghost'
                        title={t('Synchronize')}
                        disabled={syncMutation.isPending}
                        onClick={() => syncMutation.mutate(currency.code)}
                      >
                        <RefreshCw className='size-4' />
                      </Button>
                    )}
                    <Button
                      type='button'
                      size='icon'
                      variant='ghost'
                      title={t('Edit')}
                      onClick={() => openEdit(currency)}
                    >
                      <Pencil className='size-4' />
                    </Button>
                    {currency.code !== 'USD' && (
                      <Button
                        type='button'
                        size='icon'
                        variant='ghost'
                        title={t('Delete')}
                        disabled={deleteMutation.isPending}
                        onClick={() => deleteMutation.mutate(currency.code)}
                      >
                        <Trash2 className='size-4' />
                      </Button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    )
  }

  return (
    <SettingsSection title={t('Platform currencies')}>
      <div className='space-y-4'>
        <div className='flex items-center justify-between gap-3'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Choose payment currencies and define how their USD rates are maintained.'
            )}
          </p>
          <Button type='button' onClick={openCreate} className='shrink-0'>
            <Plus className='mr-2 size-4' />
            {t('Add currency')}
          </Button>
        </div>
        <div className='space-y-3 rounded-lg border p-4'>
          <div className='flex flex-wrap items-start justify-between gap-3'>
            <div>
              <h3 className='font-medium'>{t('Automatic synchronization')}</h3>
              <p className='text-muted-foreground mt-1 text-sm'>
                {t(
                  'One batch updates every currency with synchronization enabled.'
                )}
              </p>
            </div>
            <Button
              type='button'
              variant='outline'
              disabled={
                batchSyncMutation.isPending || synchronizedCurrencies.length === 0
              }
              onClick={() => batchSyncMutation.mutate()}
            >
              <RefreshCw
                className={
                  batchSyncMutation.isPending
                    ? 'mr-2 size-4 animate-spin'
                    : 'mr-2 size-4'
                }
              />
              {t('Synchronize now')}
            </Button>
          </div>
          <div className='grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end'>
            <div className='space-y-2'>
              <Label>{t('Update interval')}</Label>
              <Select
                value={syncInterval || configuredSyncInterval}
                disabled={syncConfigQuery.isLoading || updateSyncConfigMutation.isPending}
                onValueChange={(value) => value && setSyncInterval(value)}
              >
                <SelectTrigger>
                  <SelectValue>
                    {getSyncIntervalLabel(
                      syncInterval || configuredSyncInterval,
                      t
                    )}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {allowedIntervals.map((interval) => (
                    <SelectItem key={interval} value={interval}>
                      {getSyncIntervalLabel(interval, t)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Button
              type='button'
              disabled={
                syncConfigQuery.isLoading ||
                updateSyncConfigMutation.isPending ||
                syncInterval === configuredSyncInterval
              }
              onClick={() => updateSyncConfigMutation.mutate(syncInterval)}
            >
              {t('Save')}
            </Button>
          </div>
        </div>
        {renderCurrencyList()}
      </div>
      <Dialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={editing ? t('Edit currency') : t('Add currency')}
        description={t(
          'Rates are expressed as 1 USD = X units of the selected currency.'
        )}
        contentClassName='sm:max-w-lg'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setDialogOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              disabled={saveMutation.isPending}
              onClick={() => saveMutation.mutate()}
            >
              {editing ? t('Update') : t('Add')}
            </Button>
          </>
        }
      >
        <div className='space-y-4'>
          <div className='grid gap-4 sm:grid-cols-2'>
            <div className='space-y-2'>
              <Label htmlFor='platform-currency-code'>{t('Code')}</Label>
              <Input
                id='platform-currency-code'
                value={draft.code}
                disabled={Boolean(editing)}
                onChange={(event) =>
                  setDraftField('code', event.target.value.toUpperCase())
                }
                placeholder='USD'
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='platform-currency-symbol'>{t('Symbol')}</Label>
              <Input
                id='platform-currency-symbol'
                value={draft.symbol}
                onChange={(event) =>
                  setDraftField('symbol', event.target.value)
                }
                placeholder='$'
              />
            </div>
          </div>
          <div className='space-y-2'>
            <Label htmlFor='platform-currency-name'>{t('Name')}</Label>
            <Input
              id='platform-currency-name'
              value={draft.name}
              onChange={(event) => setDraftField('name', event.target.value)}
              placeholder={t('US Dollar')}
            />
          </div>
          <div className='flex items-center justify-between rounded-lg border p-3'>
            <div>
              <Label htmlFor='platform-currency-enabled'>
                {t('Available for payment methods')}
              </Label>
              <p className='text-muted-foreground text-xs'>
                {t('Disabled currencies are hidden from user payment choices.')}
              </p>
            </div>
            <Switch
              id='platform-currency-enabled'
              checked={draft.enabled}
              onCheckedChange={(checked) => setDraftField('enabled', checked)}
            />
          </div>
          <div className='flex items-center justify-between rounded-lg border p-3'>
            <div>
              <Label htmlFor='platform-currency-sync'>
                {t('Synchronize rate')}
              </Label>
              <p className='text-muted-foreground text-xs'>
                {t('Turn off to enter the USD rate manually.')}
              </p>
            </div>
            <Switch
              id='platform-currency-sync'
              checked={draft.sync_enabled}
              disabled={getSupportedSyncProviders(draft.code).length === 0}
              onCheckedChange={toggleDraftSync}
            />
          </div>
          {draft.sync_enabled ? (
            <div className='space-y-2'>
              <Label>{t('Synchronization source')}</Label>
              <Select
                value={draft.sync_provider}
                onValueChange={(value) =>
                  value && setDraftField('sync_provider', value)
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {getSupportedSyncProviders(draft.code).map((provider) => (
                    <SelectItem key={provider} value={provider}>
                      {getSyncProviderLabel(provider)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          ) : (
            <div className='space-y-2'>
              <Label htmlFor='platform-currency-rate'>
                {t('Manual rate to USD')}
              </Label>
              <Input
                id='platform-currency-rate'
                type='number'
                min='0.000001'
                step='any'
                value={draft.manual_rate_to_usd}
                onChange={(event) =>
                  setDraftField(
                    'manual_rate_to_usd',
                    Number(event.target.value)
                  )
                }
              />
              <p className='text-muted-foreground text-xs'>
                {t('1 USD = X units of this currency')}
              </p>
            </div>
          )}
        </div>
      </Dialog>
    </SettingsSection>
  )
}
