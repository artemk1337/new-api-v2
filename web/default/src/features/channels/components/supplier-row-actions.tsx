import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { DataTableRowActionMenu } from '@/components/data-table/core/row-action-menu'
import { DropdownMenuItem } from '@/components/ui/dropdown-menu'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import { handleBatchDisable, handleBatchEnable } from '../lib'
import type { SupplierRow } from '../lib/channel-utils'

export function SupplierRowActions(props: { channel: SupplierRow }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const canOperate = hasPermission(
    user,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.OPERATE
  )
  if (!canOperate) return null
  const ids = props.channel.children.map((channel) => channel.id)
  return (
    <DataTableRowActionMenu ariaLabel={t('Open menu')}>
      <DropdownMenuItem onClick={() => handleBatchEnable(ids, queryClient)}>
        {t('Enable channels on this page')}
      </DropdownMenuItem>
      <DropdownMenuItem onClick={() => handleBatchDisable(ids, queryClient)}>
        {t('Disable channels on this page')}
      </DropdownMenuItem>
    </DataTableRowActionMenu>
  )
}
