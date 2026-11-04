import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { getChannels } from '@/features/channels/api'
import { useAuthStore } from '@/stores/auth-store'

import {
  getGroupSupplierBindings,
  getSupplierChoices,
  saveSupplierBindings,
  supplierQueryKeys,
} from './api'
import { SupplierSelector } from './supplier-selector'
import type { SupplierBinding } from './types'

export function GroupSupplierBindingEditor(props: {
  groupId: number
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const isRoot = user?.role === 100
  const queryClient = useQueryClient()
  const checkboxId = useId()
  const [draft, setDraft] = useState<SupplierBinding | null>(null)
  const [includeChannels, setIncludeChannels] = useState(false)
  const bindings = useQuery({
    queryKey: supplierQueryKeys.bindings,
    queryFn: getGroupSupplierBindings,
    enabled: isRoot,
  })
  const choices = useQuery({
    queryKey: supplierQueryKeys.choices,
    queryFn: getSupplierChoices,
    enabled: isRoot,
    staleTime: 60_000,
  })
  const binding = bindings.data?.find(
    (item) => item.group_id === props.groupId
  ) ?? { supplier_id: null, profile_id: null }
  const supplier = choices.data?.find((item) => item.id === binding.supplier_id)
  const profile = supplier?.profiles.find(
    (item) => item.id === binding.profile_id
  )
  const attachedChannels = useQuery({
    queryKey: ['suppliers', 'group-channels', props.groupId],
    enabled: isRoot && draft !== null && includeChannels,
    queryFn: async () => {
      const ids: number[] = []
      let total = 1
      for (let page = 1; ids.length < total; page++) {
        const response = await getChannels({
          group: String(props.groupId),
          p: page,
          page_size: 100,
        })
        if (!response.success || !response.data) {
          throw new Error('Failed to load channels')
        }
        total = response.data.total
        if (response.data.items.length === 0 && ids.length < total) {
          throw new Error('Failed to load channels')
        }
        ids.push(...response.data.items.map((channel) => channel.id))
      }
      return ids
    },
  })
  const save = useMutation({
    mutationFn: (value: SupplierBinding) =>
      saveSupplierBindings(
        [{ group_id: props.groupId, ...value }],
        includeChannels
          ? (attachedChannels.data ?? []).map((channelId) => ({
              channel_id: channelId,
              ...value,
            }))
          : []
      ),
    onSuccess: () => {
      setDraft(null)
      void queryClient.invalidateQueries({ queryKey: supplierQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: ['channels'] })
    },
  })

  if (!isRoot) return <span>—</span>

  return (
    <>
      <Button
        size='sm'
        variant='ghost'
        disabled={props.disabled || !bindings.data || !choices.data}
        onClick={() => {
          setDraft(binding)
          setIncludeChannels(false)
        }}
        className='h-auto max-w-56 flex-col items-start py-1 text-left'
      >
        <span className='max-w-full truncate'>
          {supplier?.name ??
            (binding.supplier_id
              ? `#${binding.supplier_id}`
              : t('Default pricing'))}
        </span>
        {profile && (
          <span className='text-muted-foreground max-w-full truncate text-xs'>
            {profile.name}
          </span>
        )}
      </Button>
      {draft && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open) setDraft(null)
          }}
          title={t('Group supplier binding')}
          description={t(
            'Channels and groups must use the same supplier and price profile. Existing channel assignments must remain compatible.'
          )}
          contentHeight='auto'
          footer={
            <>
              <Button variant='outline' onClick={() => setDraft(null)}>
                {t('Cancel')}
              </Button>
              <Button
                disabled={
                  save.isPending ||
                  Boolean(draft.supplier_id && !draft.profile_id) ||
                  (includeChannels && !attachedChannels.data)
                }
                onClick={() => save.mutate(draft)}
              >
                {t('Save')}
              </Button>
            </>
          }
        >
          <div className='space-y-4'>
            <SupplierSelector value={draft} onChange={setDraft} />
            <div className='flex items-center gap-2'>
              <Checkbox
                id={checkboxId}
                checked={includeChannels}
                onCheckedChange={(checked) =>
                  setIncludeChannels(checked === true)
                }
              />
              <Label htmlFor={checkboxId}>
                {t('Rebind attached channels in the same operation')}
              </Label>
            </div>
            {includeChannels && (
              <p className='text-muted-foreground text-sm'>
                {t('Channels')}: {attachedChannels.data?.length ?? '…'}
              </p>
            )}
            {attachedChannels.isError && includeChannels && (
              <p role='alert' className='text-destructive text-sm'>
                {t('Failed to load channels')}
              </p>
            )}
            <p className='text-muted-foreground text-sm'>
              {t(
                'A multiplier of 2 charges twice the real USD cost, including cache and other billing components.'
              )}
            </p>
          </div>
        </Dialog>
      )}
    </>
  )
}
