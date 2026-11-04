import { useTranslation } from 'react-i18next'

import { useSupplierChoices } from '@/features/suppliers/use-supplier-choices'

import type { Channel } from '../types'

export function ChannelSupplierCell(props: {
  channel: Channel
  visible: boolean
}) {
  const { t } = useTranslation()
  const choices = useSupplierChoices()
  if (!props.visible) return <span>••••</span>
  if (!props.channel.supplier_id) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('Default pricing')}
      </span>
    )
  }
  const supplier = choices.data?.find(
    (item) => item.id === props.channel.supplier_id
  )
  const profile = supplier?.profiles.find(
    (item) => item.id === props.channel.supplier_profile_id
  )
  return (
    <div className='max-w-48 text-xs'>
      <div className='truncate'>
        {supplier?.name ?? `#${props.channel.supplier_id}`}
      </div>
      {props.channel.supplier_profile_id && (
        <div className='text-muted-foreground truncate'>
          {profile?.name ?? `#${props.channel.supplier_profile_id}`}
        </div>
      )}
    </div>
  )
}
