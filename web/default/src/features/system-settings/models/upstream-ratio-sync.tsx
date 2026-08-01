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
import { CheckSquare, RefreshCcw } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  fetchUpstreamRatios,
  applyPricingSyncPatches,
  getPricingSyncConfig,
  getPricingSyncConfigQuietly,
  getUpstreamChannels,
  updatePricingSyncConfig,
} from '../api'
import type {
  DifferencesMap,
  PricingSyncModelPreference,
  PricingSyncModelState,
  RatioType,
  PricingSyncConfig,
  PricingSyncConfigResponse,
  UpstreamChannel,
  UpstreamConfig,
} from '../types'
import {
  ConflictConfirmDialog,
  type ConflictItem,
} from './conflict-confirm-dialog'
import {
  DEFAULT_ENDPOINT,
  MODELS_DEV_PRESET_ENDPOINT,
  MODELS_DEV_PRESET_ID,
  OFFICIAL_CHANNEL_ENDPOINT,
  OFFICIAL_CHANNEL_ID,
  OPENROUTER_CHANNEL_TYPE,
  OPENROUTER_ENDPOINT,
} from './constants'
import { PricingSyncSources } from './pricing-sync-sources'
import {
  buildPricingSyncPatches,
  RATIO_SYNC_FIELDS,
  getPricingSyncErrorMessage,
  getPreferredSyncField,
  getRunnablePricingSyncSources,
  isPricingSyncVersionConflict,
  pricingSyncPreferencesForModels,
  rebasePricingSyncConfigDraft,
  splitPricingSyncDifferences,
  type ResolutionsMap,
} from './upstream-ratio-sync-helpers'
import { UpstreamRatioSyncTable } from './upstream-ratio-sync-table'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type UpstreamRatioSyncProps = {
  modelRatios: {
    ModelPrice: string
    ModelRatio: string
    CompletionRatio: string
    CacheRatio: string
    CreateCacheRatio: string
    ImageRatio: string
    AudioRatio: string
    AudioCompletionRatio: string
    'billing_setting.billing_mode': string
    'billing_setting.billing_expr': string
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// The two synthesized presets always carry stable negative IDs assigned by
// `controller/ratio_sync.go`; matching by ID alone is sufficient and avoids
// fragile name/base_url comparisons.
function getDefaultEndpointForChannel(channel: UpstreamChannel): string {
  if (channel.id === MODELS_DEV_PRESET_ID) return MODELS_DEV_PRESET_ENDPOINT
  if (channel.id === OFFICIAL_CHANNEL_ID) return OFFICIAL_CHANNEL_ENDPOINT
  if (channel.type === OPENROUTER_CHANNEL_TYPE) return OPENROUTER_ENDPOINT
  return DEFAULT_ENDPOINT
}

function getBillingCategory(ratioType: string): 'price' | 'ratio' | 'tiered' {
  if (ratioType === 'model_price') {
    return 'price'
  }
  if (ratioType === 'billing_mode' || ratioType === 'billing_expr') {
    return 'tiered'
  }
  return 'ratio'
}

function parseJsonRecord<T>(raw: string | undefined | null): Record<string, T> {
  try {
    return JSON.parse(raw || '{}') as Record<string, T>
  } catch {
    return {}
  }
}

function deleteResolutionField(
  res: ResolutionsMap,
  model: string,
  ratioType: string
): ResolutionsMap {
  if (!res[model]) return res
  const newModelRes = { ...res[model] }
  delete newModelRes[ratioType]
  if (ratioType === 'billing_expr') delete newModelRes['billing_mode']
  if (ratioType === 'billing_mode') delete newModelRes['billing_expr']
  const next = { ...res }
  if (Object.keys(newModelRes).length === 0) {
    delete next[model]
  } else {
    next[model] = newModelRes
  }
  return next
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function UpstreamRatioSync({ modelRatios }: UpstreamRatioSyncProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [conflictDialogOpen, setConflictDialogOpen] = useState(false)
  const [syncConfig, setSyncConfig] = useState<PricingSyncConfig>({
    strategy: 'highest',
    sources: [],
    version: 0,
  })
  const [configDirty, setConfigDirty] = useState(false)
  const persistedConfigRef = useRef<PricingSyncConfig | null>(null)
  const [differences, setDifferences] = useState<DifferencesMap>({})
  const [modelStates, setModelStates] = useState<
    Record<string, PricingSyncModelState>
  >({})
  const [pricingSnapshotVersion, setPricingSnapshotVersion] = useState<
    number | null
  >(null)
  const [resolutions, setResolutions] = useState<ResolutionsMap>({})
  const [conflictItems, setConflictItems] = useState<ConflictItem[]>([])
  const [confirmLoading, setConfirmLoading] = useState(false)
  const [pendingAutoModels, setPendingAutoModels] = useState<string[]>([])

  const { data: channelsData } = useQuery({
    queryKey: ['upstream-channels'],
    queryFn: getUpstreamChannels,
  })

  const { data: syncConfigData } = useQuery({
    queryKey: ['pricing-sync-config'],
    queryFn: getPricingSyncConfig,
  })

  // Memoize the channels list so the effect below only re-runs when the query
  // data actually changes, instead of on every render (the `|| []` fallback
  // would otherwise produce a new array reference each render).
  const channels = useMemo(() => channelsData?.data ?? [], [channelsData?.data])

  useEffect(() => {
    if (!syncConfigData?.success || configDirty) return
    persistedConfigRef.current = syncConfigData.data
    setSyncConfig(syncConfigData.data)
  }, [configDirty, syncConfigData])

  const updatePricingSyncVersion = useCallback(
    (version: number) => {
      setPricingSnapshotVersion(version)
      setSyncConfig((current) => ({ ...current, version }))
      if (persistedConfigRef.current) {
        persistedConfigRef.current = {
          ...persistedConfigRef.current,
          version,
        }
      }
      queryClient.setQueryData<PricingSyncConfigResponse>(
        ['pricing-sync-config'],
        (current) =>
          current ? { ...current, data: { ...current.data, version } } : current
      )
    },
    [queryClient]
  )

  const clearPricingSnapshot = useCallback(() => {
    setPricingSnapshotVersion(null)
    setDifferences({})
    setModelStates({})
    setResolutions({})
    setConflictItems([])
    setConflictDialogOpen(false)
    setPendingAutoModels([])
  }, [])

  const fetchLatestPricingSyncConfig = useCallback(async () => {
    await queryClient.invalidateQueries({
      queryKey: ['pricing-sync-config'],
      refetchType: 'none',
    })
    const latest = await queryClient.fetchQuery({
      queryKey: ['pricing-sync-config'],
      queryFn: getPricingSyncConfigQuietly,
    })
    if (!latest.success) {
      throw new Error(latest.message || t('Failed to load'))
    }
    return latest.data
  }, [queryClient, t])

  const fetchMutation = useMutation({
    mutationFn: fetchUpstreamRatios,
    onSuccess: (data) => {
      if (!data.success) {
        toast.error(data.message || t('Failed to fetch upstream prices'))
        return
      }

      const {
        differences: diffs,
        model_states,
        config_version,
        test_results,
      } = data.data

      const errorResults = test_results.filter((r) => r.status === 'error')
      if (errorResults.length > 0) {
        const errorMsg = errorResults
          .map((r) => `${r.name}: ${r.error}`)
          .join(', ')
        toast.warning(t('Some channels failed: {{errorMsg}}', { errorMsg }))
      }

      const warningResults = test_results.filter(
        (r) => (r.warnings?.length ?? 0) > 0
      )
      if (warningResults.length > 0) {
        const warningMsg = warningResults
          .map((r) => `${r.name}: ${r.warnings?.length ?? 0}`)
          .join('; ')
        toast.warning(
          t('Unsupported or invalid pricing skipped: {{warningMsg}}', {
            warningMsg,
          })
        )
      }

      setDifferences(diffs)
      setModelStates(model_states ?? {})
      setPricingSnapshotVersion(config_version)
      setResolutions({})

      if (Object.keys(diffs).length === 0) {
        toast.success(t('No price differences found'))
      } else {
        toast.success(t('Upstream prices fetched successfully'))
      }
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to fetch upstream prices'))
    },
  })

  const { mutate: syncMutate, isPending: isSyncPending } = useMutation({
    mutationFn: ({
      patches,
      preferences,
      expectedVersion,
    }: {
      patches: Record<
        string,
        { set?: Record<string, number | string>; delete?: string[] }
      >
      preferences: PricingSyncModelPreference[]
      expectedVersion: number
    }) => applyPricingSyncPatches(patches, preferences, expectedVersion),
    onError: (error: unknown) => {
      if (isPricingSyncVersionConflict(error)) {
        clearPricingSnapshot()
        toast.error(
          t(
            'Pricing settings changed. Check prices again before applying changes.'
          )
        )
        return
      }

      toast.error(getPricingSyncErrorMessage(error, t('Failed to sync prices')))
    },
  })

  const automaticSources = useMemo(
    () =>
      getRunnablePricingSyncSources(
        channels,
        syncConfigData?.success ? syncConfigData.data : undefined
      ),
    [channels, syncConfigData]
  )
  const hasRunnableSources =
    automaticSources.length > 0 && pricingSnapshotVersion !== null

  const differenceGroups = useMemo(
    () => splitPricingSyncDifferences(differences, modelStates),
    [differences, modelStates]
  )

  const handleFetchConfiguredSources = () => {
    const selectedChannels = channels.filter((ch) =>
      syncConfig.sources.some(
        (source) => source.channel_id === ch.id && source.enabled
      )
    )

    if (selectedChannels.length === 0) {
      toast.warning(t('Please select at least one channel'))
      return
    }

    const upstreams: UpstreamConfig[] = selectedChannels.map((ch) => ({
      id: ch.id,
      name: ch.name,
      base_url: ch.base_url,
      endpoint:
        syncConfig.sources.find((source) => source.channel_id === ch.id)
          ?.endpoint || getDefaultEndpointForChannel(ch),
    }))

    fetchMutation.mutate({ upstreams, timeout: 10 })
  }

  const saveConfigMutation = useMutation({
    mutationFn: () => updatePricingSyncConfig(syncConfig),
    onSuccess: async () => {
      clearPricingSnapshot()
      toast.success(t('Synchronization settings saved'))
      try {
        const latest = await fetchLatestPricingSyncConfig()
        persistedConfigRef.current = latest
        setSyncConfig(latest)
        setConfigDirty(false)
      } catch (error) {
        toast.error(getPricingSyncErrorMessage(error, t('Failed to load')))
      }
    },
    onError: async (error: unknown) => {
      if (isPricingSyncVersionConflict(error)) {
        clearPricingSnapshot()
        const base = persistedConfigRef.current
        try {
          const latest = await fetchLatestPricingSyncConfig()
          setSyncConfig((current) =>
            base
              ? rebasePricingSyncConfigDraft(base, current, latest)
              : { ...current, version: latest.version }
          )
          persistedConfigRef.current = latest
        } catch (refreshError) {
          toast.error(
            getPricingSyncErrorMessage(refreshError, t('Failed to load'))
          )
        }
        toast.error(
          t(
            'Pricing settings changed. Check prices again before applying changes.'
          )
        )
        return
      }
      toast.error(
        error instanceof Error ? error.message : t('Failed to save settings')
      )
    },
  })

  const handleSelectValue = useCallback(
    (
      model: string,
      ratioType: RatioType,
      value: number | string,
      sourceName: string
    ) => {
      const modelDiffs = differences[model]

      // Prefer billing_expr over individual ratio fields when available
      const preferredType = sourceName
        ? getPreferredSyncField(modelDiffs || {}, ratioType, sourceName)
        : ratioType
      const preferredValue =
        preferredType === ratioType
          ? value
          : (modelDiffs?.[preferredType]?.upstreams?.[sourceName] ?? value)

      const finalType = preferredType
      const finalValue = preferredValue as number | string
      const category = getBillingCategory(finalType)

      setResolutions((prev) => {
        const newModelRes = { ...prev[model] }

        // Clear conflicting categories
        Object.keys(newModelRes).forEach((rt) => {
          if (
            category !== 'tiered' &&
            getBillingCategory(rt) !== 'tiered' &&
            getBillingCategory(rt) !== category
          ) {
            delete newModelRes[rt]
          }
        })

        newModelRes[finalType] = finalValue

        // When selecting a tiered field, auto-populate paired fields from the same source
        if (category === 'tiered' && sourceName && modelDiffs) {
          const modeVal = modelDiffs.billing_mode?.upstreams?.[sourceName]
          const exprVal = modelDiffs.billing_expr?.upstreams?.[sourceName]
          if (modeVal !== undefined && modeVal !== null && modeVal !== 'same') {
            newModelRes['billing_mode'] = modeVal
          } else if (finalType === 'billing_expr') {
            newModelRes['billing_mode'] = 'tiered_expr'
          }
          if (exprVal !== undefined && exprVal !== null && exprVal !== 'same') {
            newModelRes['billing_expr'] = exprVal
          }
        }

        return { ...prev, [model]: newModelRes }
      })
    },
    [differences]
  )

  const handleUnselectValue = useCallback(
    (model: string, ratioType: RatioType) => {
      setResolutions((prev) => deleteResolutionField(prev, model, ratioType))
    },
    []
  )

  const parsedRatios = useMemo(() => {
    return {
      ModelRatio: parseJsonRecord<number>(modelRatios.ModelRatio),
      CompletionRatio: parseJsonRecord<number>(modelRatios.CompletionRatio),
      CacheRatio: parseJsonRecord<number>(modelRatios.CacheRatio),
      CreateCacheRatio: parseJsonRecord<number>(modelRatios.CreateCacheRatio),
      ImageRatio: parseJsonRecord<number>(modelRatios.ImageRatio),
      AudioRatio: parseJsonRecord<number>(modelRatios.AudioRatio),
      AudioCompletionRatio: parseJsonRecord<number>(
        modelRatios.AudioCompletionRatio
      ),
      ModelPrice: parseJsonRecord<number>(modelRatios.ModelPrice),
      'billing_setting.billing_mode': parseJsonRecord<string>(
        modelRatios['billing_setting.billing_mode']
      ),
      'billing_setting.billing_expr': parseJsonRecord<string>(
        modelRatios['billing_setting.billing_expr']
      ),
    }
  }, [modelRatios])

  type ParsedRatios = typeof parsedRatios

  const getLocalBillingCategory = (
    model: string,
    currentRatios: ParsedRatios
  ): 'price' | 'ratio' | null => {
    if (currentRatios.ModelPrice[model] !== undefined) return 'price'
    if (
      currentRatios.ModelRatio[model] !== undefined ||
      currentRatios.CompletionRatio[model] !== undefined ||
      currentRatios.CacheRatio[model] !== undefined ||
      currentRatios.CreateCacheRatio[model] !== undefined ||
      currentRatios.ImageRatio[model] !== undefined ||
      currentRatios.AudioRatio[model] !== undefined ||
      currentRatios.AudioCompletionRatio[model] !== undefined
    ) {
      return 'ratio'
    }
    return null
  }

  const performSync = useCallback(async (): Promise<boolean> => {
    if (pricingSnapshotVersion === null) return false

    const patches = buildPricingSyncPatches(resolutions)
    const preferences = pricingSyncPreferencesForModels(
      Object.keys(resolutions),
      modelStates
    ).map(({ model_name, mode, channel_id }) => ({
      model_name,
      mode,
      channel_id,
    }))

    return new Promise<boolean>((resolve) => {
      syncMutate(
        {
          patches,
          preferences,
          expectedVersion: pricingSnapshotVersion,
        },
        {
          onSuccess: (data) => {
            updatePricingSyncVersion(data.data.config_version)
            toast.success(t('Prices synced successfully'))
            void queryClient.invalidateQueries({ queryKey: ['system-options'] })
            void queryClient.invalidateQueries({
              queryKey: ['pricing-sync-model-state'],
            })
            setDifferences((current) => {
              const next = { ...current }
              Object.entries(resolutions).forEach(([modelName, ratios]) => {
                Object.keys(ratios).forEach((ratioType) => {
                  delete next[modelName]?.[ratioType as RatioType]
                  if (
                    next[modelName] &&
                    Object.keys(next[modelName]).length === 0
                  ) {
                    delete next[modelName]
                  }
                })
              })
              return next
            })
            setResolutions({})
            resolve(true)
          },
          onError: () => resolve(false),
        }
      )
    })
  }, [
    modelStates,
    pricingSnapshotVersion,
    queryClient,
    resolutions,
    syncMutate,
    t,
    updatePricingSyncVersion,
  ])

  const enableAutoForModels = useCallback(
    (modelNames: string[], channelID: number) => {
      if (pricingSnapshotVersion === null) return

      const preferences: PricingSyncModelPreference[] = modelNames.map(
        (model_name) => ({
          model_name,
          mode: channelID === 0 ? 'general' : 'channel',
          channel_id: channelID,
        })
      )
      syncMutate(
        {
          patches: {},
          preferences,
          expectedVersion: pricingSnapshotVersion,
        },
        {
          onSuccess: (data) => {
            updatePricingSyncVersion(data.data.config_version)
            setModelStates((current) => {
              const next = { ...current }
              modelNames.forEach((modelName) => {
                next[modelName] = {
                  ...next[modelName],
                  model_name: modelName,
                  mode: channelID === 0 ? 'general' : 'channel',
                  channel_id: channelID,
                  status: 'stale',
                }
              })
              return next
            })
            setResolutions((current) => {
              const next = { ...current }
              modelNames.forEach((modelName) => delete next[modelName])
              return next
            })
            setPendingAutoModels([])
            modelNames.forEach((modelName) => {
              void queryClient.invalidateQueries({
                queryKey: ['pricing-sync-model-state', modelName],
              })
            })
            toast.success(
              t(
                'Automatic price updates will start after upstream confirmation.'
              )
            )
          },
        }
      )
    },
    [
      pricingSnapshotVersion,
      queryClient,
      syncMutate,
      t,
      updatePricingSyncVersion,
    ]
  )

  const findSourceChannel = (
    model: string,
    ratioType: RatioType,
    value: number | string
  ): string => {
    const upMap = differences[model]?.[ratioType]?.upstreams
    if (!upMap) return 'Unknown'
    const entry = Object.entries(upMap).find(([, v]) => v === value)
    return entry ? entry[0] : 'Unknown'
  }

  const handleApplySync = () => {
    const currentRatios = parsedRatios
    const conflicts: ConflictItem[] = []

    const fixedPriceLabel = t('Fixed price')
    const modelRatioLabel = t('Model ratio')
    const completionRatioLabel = t('Completion ratio')

    Object.entries(resolutions).forEach(([model, ratios]) => {
      const localCat = getLocalBillingCategory(model, currentRatios)
      const selectedTypes = Object.keys(ratios)
      let newCat: 'price' | 'ratio' | 'tiered'
      if ('model_price' in ratios) {
        newCat = 'price'
      } else if (RATIO_SYNC_FIELDS.some((rt) => selectedTypes.includes(rt))) {
        newCat = 'ratio'
      } else {
        newCat = 'tiered'
      }

      if (localCat && newCat !== 'tiered' && localCat !== newCat) {
        const currentDesc =
          localCat === 'price'
            ? `${fixedPriceLabel}: ${currentRatios.ModelPrice[model]}`
            : `${modelRatioLabel}: ${currentRatios.ModelRatio[model] ?? '-'}\n${completionRatioLabel}: ${currentRatios.CompletionRatio[model] ?? '-'}`

        const newDesc =
          newCat === 'price'
            ? `${fixedPriceLabel}: ${ratios.model_price}`
            : `${modelRatioLabel}: ${ratios.model_ratio ?? '-'}\n${completionRatioLabel}: ${ratios.completion_ratio ?? '-'}`

        const channelNames = selectedTypes
          .map((rt) => findSourceChannel(model, rt as RatioType, ratios[rt]))
          .filter((v, idx, arr) => arr.indexOf(v) === idx)
          .join(', ')

        conflicts.push({
          channel: channelNames,
          model,
          current: currentDesc,
          newVal: newDesc,
        })
      }
    })

    if (conflicts.length > 0) {
      setConflictItems(conflicts)
      setConflictDialogOpen(true)
      return
    }

    toast.info(t('Syncing prices, please wait...'))
    performSync()
  }

  const handleConfirmConflict = async () => {
    setConfirmLoading(true)
    try {
      const success = await performSync()
      if (success) {
        setConflictDialogOpen(false)
      }
    } finally {
      setConfirmLoading(false)
    }
  }

  const hasSelections = Object.keys(resolutions).length > 0
  const isPricingSyncReady =
    channelsData?.success === true &&
    syncConfigData?.success === true &&
    persistedConfigRef.current !== null
  const isLoading =
    !isPricingSyncReady ||
    fetchMutation.isPending ||
    isSyncPending ||
    confirmLoading ||
    saveConfigMutation.isPending

  return (
    <div className='space-y-4'>
      <PricingSyncSources
        channels={channels}
        value={syncConfig}
        disabled={isLoading}
        onChange={(value) => {
          setSyncConfig(value)
          setConfigDirty(true)
        }}
        onSave={() => saveConfigMutation.mutate()}
      />

      <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex flex-col gap-2 sm:flex-row'>
          <Button onClick={handleFetchConfiguredSources} disabled={isLoading}>
            <RefreshCcw className='mr-2 h-4 w-4' />
            {t('Check prices')}
          </Button>
          <Button
            variant='secondary'
            onClick={handleApplySync}
            disabled={!hasSelections || isLoading}
          >
            {(isSyncPending || confirmLoading) && (
              <span className='mr-2 h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent' />
            )}
            <CheckSquare className='mr-2 h-4 w-4' />
            {t('Apply Sync')}
          </Button>
        </div>
      </div>

      <Tabs defaultValue='automatic'>
        <TabsList className='grid w-full grid-cols-2'>
          <TabsTrigger value='automatic'>
            {t('Automatic updates ({{count}})', {
              count: Object.keys(differenceGroups.automatic).length,
            })}
          </TabsTrigger>
          <TabsTrigger value='manual'>
            {t('Manual prices ({{count}})', {
              count: Object.keys(differenceGroups.manual).length,
            })}
          </TabsTrigger>
        </TabsList>
        <TabsContent value='automatic' className='mt-4'>
          <UpstreamRatioSyncTable
            differences={differenceGroups.automatic}
            resolutions={resolutions}
            isDisabled={isLoading}
            isSyncing={fetchMutation.isPending}
            onSelectValue={handleSelectValue}
            onUnselectValue={handleUnselectValue}
          />
        </TabsContent>
        <TabsContent value='manual' className='mt-4 space-y-3'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'These prices differ upstream but are protected from automatic updates.'
            )}
          </p>
          {!hasRunnableSources && (
            <p className='text-muted-foreground text-sm'>
              {t(
                'Save at least one enabled source with an automatic update interval before enabling auto.'
              )}
            </p>
          )}
          <UpstreamRatioSyncTable
            differences={differenceGroups.manual}
            resolutions={resolutions}
            isDisabled={isLoading}
            isSyncing={fetchMutation.isPending}
            onSelectValue={handleSelectValue}
            onUnselectValue={handleUnselectValue}
            autoSources={automaticSources}
            onEnableAuto={
              hasRunnableSources
                ? (modelName, channelID) =>
                    enableAutoForModels([modelName], channelID)
                : undefined
            }
            onBulkEnableAuto={
              hasRunnableSources ? setPendingAutoModels : undefined
            }
          />
        </TabsContent>
      </Tabs>

      <ConflictConfirmDialog
        open={conflictDialogOpen}
        onOpenChange={setConflictDialogOpen}
        conflicts={conflictItems}
        onConfirm={handleConfirmConflict}
        isLoading={confirmLoading}
      />
      <AlertDialog
        open={pendingAutoModels.length > 0}
        onOpenChange={(open) => !open && setPendingAutoModels([])}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Enable automatic price updates?')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Enable automatic updates for {{count}} shown models. Prices will update after upstream confirmation.',
                {
                  count: pendingAutoModels.length,
                }
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isLoading}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={isLoading}
              onClick={() => enableAutoForModels(pendingAutoModels, 0)}
            >
              {t('Enable auto')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
