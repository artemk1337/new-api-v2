import { useQuery } from '@tanstack/react-query'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'

import { getSupplierChoices, supplierQueryKeys } from './api'
import type { SupplierBinding } from './types'

interface SupplierSelectorProps {
  value: SupplierBinding
  onChange: (value: SupplierBinding) => void
  disabled?: boolean
}

export function SupplierSelector(props: SupplierSelectorProps) {
  const { t } = useTranslation()
  const id = useId()
  const choices = useQuery({
    queryKey: supplierQueryKeys.choices,
    queryFn: getSupplierChoices,
    enabled: !props.disabled,
    staleTime: 60_000,
  })
  const supplier = choices.data?.find(
    (item) => item.id === props.value.supplier_id
  )

  return (
    <div className='grid gap-3 sm:grid-cols-2'>
      <div className='grid min-w-0 gap-2'>
        <Label htmlFor={`${id}-supplier`}>{t('Supplier')}</Label>
        <NativeSelect
          id={`${id}-supplier`}
          className='w-full'
          value={props.value.supplier_id ?? ''}
          disabled={props.disabled || choices.isLoading || choices.isError}
          onChange={(event) =>
            props.onChange({
              supplier_id: Number(event.target.value) || null,
              profile_id: null,
            })
          }
        >
          <option value=''>{t('Default pricing')}</option>
          {!supplier && props.value.supplier_id && (
            <option value={props.value.supplier_id}>
              #{props.value.supplier_id}
            </option>
          )}
          {choices.data?.map((item) => (
            <option key={item.id} value={item.id} disabled={!item.enabled}>
              {item.name}
            </option>
          ))}
        </NativeSelect>
      </div>
      <div className='grid min-w-0 gap-2'>
        <Label htmlFor={`${id}-profile`}>{t('Price profile')}</Label>
        <NativeSelect
          id={`${id}-profile`}
          className='w-full'
          value={props.value.profile_id ?? ''}
          disabled={props.disabled || !supplier || choices.isError}
          onChange={(event) =>
            props.onChange({
              supplier_id: props.value.supplier_id,
              profile_id: Number(event.target.value) || null,
            })
          }
        >
          <option value=''>
            {supplier ? t('Select price profile') : t('Default pricing')}
          </option>
          {!supplier?.profiles.some(
            (item) => item.id === props.value.profile_id
          ) &&
            props.value.profile_id && (
              <option value={props.value.profile_id}>
                #{props.value.profile_id}
              </option>
            )}
          {supplier?.profiles.map((item) => (
            <option key={item.id} value={item.id} disabled={!item.enabled}>
              {item.name}
            </option>
          ))}
        </NativeSelect>
      </div>
      {choices.isError && (
        <p role='alert' className='text-destructive text-sm sm:col-span-2'>
          {t('Failed to load suppliers')}
        </p>
      )}
    </div>
  )
}
