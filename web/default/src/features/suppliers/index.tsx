import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { useAuthStore } from '@/stores/auth-store'

import { deleteSupplier, getSuppliers, supplierQueryKeys } from './api'
import { SupplierMigrationPreview } from './migration-preview'
import { SupplierProfilesList } from './profiles-list'
import { SupplierDialog } from './supplier-dialog'
import type { Supplier } from './types'

export function SupplierSettings() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const isRoot = user?.role === 100
  const queryClient = useQueryClient()
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [edit, setEdit] = useState<Supplier | 'new' | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [showMigration, setShowMigration] = useState(false)
  const query = useQuery({
    queryKey: supplierQueryKeys.all,
    queryFn: getSuppliers,
    enabled: isRoot,
  })
  const supplier =
    query.data?.find((item) => item.id === selectedId) ?? query.data?.[0]
  const deletion = useMutation({
    mutationFn: (id: number) => deleteSupplier(id),
    onSuccess: () => {
      setConfirmDelete(false)
      void queryClient.invalidateQueries({ queryKey: supplierQueryKeys.all })
    },
  })

  if (!isRoot) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('No permission to perform this action')}
      </p>
    )
  }

  return (
    <Card>
      <CardHeader className='border-b'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <CardTitle>{t('Suppliers')}</CardTitle>
          <div className='flex flex-wrap gap-2'>
            <Button
              size='sm'
              variant='outline'
              onClick={() => setShowMigration(true)}
            >
              {t('Migration preview')}
            </Button>
            <Button size='sm' onClick={() => setEdit('new')}>
              {t('Add supplier')}
            </Button>
          </div>
        </div>
        <CardDescription>
          {t(
            'Supplier prices are isolated by profile. Retail group multipliers apply to real USD cost.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-6 pt-6'>
        {query.isLoading && (
          <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
        )}
        {query.isError && (
          <p role='alert' className='text-destructive text-sm'>
            {t('Failed to load suppliers')}
          </p>
        )}
        {query.data?.length === 0 && (
          <p className='text-muted-foreground text-sm'>
            {t('No suppliers yet')}
          </p>
        )}
        {supplier && (
          <>
            <div className='flex flex-wrap items-end gap-3'>
              <div className='grid min-w-48 flex-1 gap-2'>
                <Label htmlFor='supplier-settings-selected'>
                  {t('Supplier')}
                </Label>
                <NativeSelect
                  id='supplier-settings-selected'
                  className='w-full'
                  value={supplier.id}
                  onChange={(event) =>
                    setSelectedId(Number(event.target.value))
                  }
                >
                  {query.data?.map((item) => (
                    <option value={item.id} key={item.id}>
                      {item.name}
                      {item.enabled ? '' : ` (${t('Disabled')})`}
                    </option>
                  ))}
                </NativeSelect>
              </div>
              <Button variant='outline' onClick={() => setEdit(supplier)}>
                {t('Edit supplier')}
              </Button>
              <Button variant='outline' onClick={() => setConfirmDelete(true)}>
                {t('Delete')}
              </Button>
            </div>
            <SupplierProfilesList key={supplier.id} supplier={supplier} />
          </>
        )}
      </CardContent>
      {edit && (
        <SupplierDialog
          value={edit === 'new' ? undefined : edit}
          onClose={() => setEdit(null)}
        />
      )}
      {showMigration && (
        <SupplierMigrationPreview onClose={() => setShowMigration(false)} />
      )}
      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={t('Delete supplier?')}
        desc={t(
          'Only suppliers without profiles, groups, or channels can be deleted.'
        )}
        destructive
        isLoading={deletion.isPending}
        confirmText={t('Delete')}
        handleConfirm={() => {
          if (supplier) deletion.mutate(supplier.id)
        }}
      />
    </Card>
  )
}
