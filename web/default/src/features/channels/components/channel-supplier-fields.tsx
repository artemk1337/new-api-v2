import { useWatch, type UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { SupplierSelector } from '@/features/suppliers/supplier-selector'

import type { ChannelFormValues } from '../lib/channel-form'

export function ChannelSupplierFields(props: {
  form: UseFormReturn<ChannelFormValues>
  disabled: boolean
}) {
  const { t } = useTranslation()
  const supplierId = useWatch({
    control: props.form.control,
    name: 'supplier_id',
  })
  const profileId = useWatch({
    control: props.form.control,
    name: 'supplier_profile_id',
  })

  return (
    <div className='space-y-2 rounded-lg border p-4'>
      <SupplierSelector
        value={{
          supplier_id: supplierId ?? null,
          profile_id: profileId ?? null,
        }}
        disabled={props.disabled}
        onChange={(value) => {
          props.form.setValue('supplier_id', value.supplier_id, {
            shouldDirty: true,
            shouldValidate: true,
          })
          props.form.setValue('supplier_profile_id', value.profile_id, {
            shouldDirty: true,
            shouldValidate: true,
          })
        }}
      />
      <p className='text-muted-foreground text-xs'>
        {t(
          'Channels and groups must use the same supplier and price profile. Existing channel assignments must remain compatible.'
        )}
      </p>
      {props.form.formState.errors.supplier_profile_id && (
        <p role='alert' className='text-destructive text-xs'>
          {t('Select a supplier and price profile together')}
        </p>
      )}
    </div>
  )
}
