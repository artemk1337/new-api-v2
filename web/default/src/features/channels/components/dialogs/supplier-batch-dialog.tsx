import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  getGroupSupplierBindings,
  saveSupplierBindings,
  supplierQueryKeys,
} from '@/features/suppliers/api'
import { groupMatchesSupplier } from '@/features/suppliers/bindings'
import { SupplierSelector } from '@/features/suppliers/supplier-selector'
import type { SupplierBinding } from '@/features/suppliers/types'

import { getGroups } from '../../api'
import { channelsQueryKeys } from '../../lib'

export function SupplierBatchDialog(props: {
  ids: number[]
  onClose: () => void
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [binding, setBinding] = useState<SupplierBinding>({
    supplier_id: null,
    profile_id: null,
  })
  const [groups, setGroups] = useState<string[]>([])
  const allGroups = useQuery({ queryKey: ['groups'], queryFn: getGroups })
  const bindings = useQuery({
    queryKey: supplierQueryKeys.bindings,
    queryFn: getGroupSupplierBindings,
  })
  const options = (allGroups.data?.data ?? [])
    .filter((group) => {
      return groupMatchesSupplier(group.id, bindings.data ?? [], binding)
    })
    .map((group) => ({ label: group.name, value: String(group.id) }))
  const save = useMutation({
    mutationFn: () =>
      saveSupplierBindings(
        [],
        props.ids.map((id) => ({
          channel_id: id,
          ...binding,
          group: groups.join(','),
        }))
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: channelsQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: supplierQueryKeys.all })
      props.onSuccess()
    },
  })

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={t('Set supplier for selected channels')}
      description={t(
        'Supplier, profile, and pricing groups are updated together. Only compatible groups are available.'
      )}
      contentHeight='auto'
      footer={
        <>
          <Button variant='outline' onClick={props.onClose}>
            {t('Cancel')}
          </Button>
          <Button
            disabled={
              save.isPending ||
              !bindings.data ||
              groups.length === 0 ||
              Boolean(binding.supplier_id && !binding.profile_id)
            }
            onClick={() => save.mutate()}
          >
            {t('Save')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <SupplierSelector
          value={binding}
          onChange={(value) => {
            setBinding(value)
            setGroups([])
          }}
        />
        <div className='grid gap-2'>
          <Label>{t('Groups')}</Label>
          <MultiSelect
            options={options}
            selected={groups}
            onChange={setGroups}
            placeholder={t('Select groups')}
          />
        </div>
        {bindings.isError && (
          <p role='alert' className='text-destructive text-sm'>
            {t('Failed to load suppliers')}
          </p>
        )}
      </div>
    </Dialog>
  )
}
