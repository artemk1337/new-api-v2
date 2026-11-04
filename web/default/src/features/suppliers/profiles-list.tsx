import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

import {
  deleteSupplierProfile,
  getSupplierProfiles,
  supplierQueryKeys,
} from './api'
import { ProfileDialog } from './profile-dialog'
import { TariffsDialog } from './tariffs-dialog'
import type { Supplier, SupplierPricingProfile } from './types'

export function SupplierProfilesList(props: { supplier: Supplier }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [edit, setEdit] = useState<SupplierPricingProfile | 'new' | null>(null)
  const [tariffs, setTariffs] = useState<SupplierPricingProfile | null>(null)
  const [remove, setRemove] = useState<SupplierPricingProfile | null>(null)
  const profiles = useQuery({
    queryKey: supplierQueryKeys.profiles(props.supplier.id),
    queryFn: () => getSupplierProfiles(props.supplier.id),
  })
  const deletion = useMutation({
    mutationFn: (id: number) => deleteSupplierProfile(id),
    onSuccess: () => {
      setRemove(null)
      void queryClient.invalidateQueries({ queryKey: supplierQueryKeys.all })
    },
  })

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h3 className='font-medium'>{t('Price profiles')}</h3>
          <p className='text-muted-foreground text-sm'>
            {t('Use a separate profile for each upstream resource group.')}
          </p>
        </div>
        <Button variant='outline' size='sm' onClick={() => setEdit('new')}>
          {t('Add price profile')}
        </Button>
      </div>
      {profiles.isLoading && (
        <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
      )}
      {profiles.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Failed to load prices')}
        </p>
      )}
      {profiles.data?.length === 0 && (
        <p className='text-muted-foreground rounded-lg border border-dashed p-5 text-sm'>
          {t('No price profiles yet')}
        </p>
      )}
      {profiles.data?.map((profile) => (
        <div key={profile.id} className='space-y-3 rounded-lg border p-4'>
          <div className='flex flex-col justify-between gap-3 sm:flex-row'>
            <div className='min-w-0'>
              <div className='flex flex-wrap items-center gap-2'>
                <span className='font-medium'>{profile.name}</span>
                <Badge variant='outline'>
                  {profile.enabled ? t('Enabled') : t('Disabled')}
                </Badge>
              </div>
              <p className='text-muted-foreground mt-1 text-xs'>
                {profile.code} · {profile.upstream_group || '—'} ·{' '}
                {t('Real USD per provider credit')}: {profile.usd_per_credit}
              </p>
            </div>
            <div className='flex flex-wrap gap-2'>
              <Button
                size='sm'
                variant='outline'
                onClick={() => setTariffs(profile)}
              >
                {t('Profile prices')}
              </Button>
              <Button
                size='sm'
                variant='ghost'
                onClick={() => setEdit(profile)}
              >
                {t('Edit')}
              </Button>
              <Button
                size='sm'
                variant='ghost'
                onClick={() => setRemove(profile)}
              >
                {t('Delete')}
              </Button>
            </div>
          </div>
          {profile.last_error && (
            <p className='text-destructive text-xs break-words'>
              {profile.last_error}
            </p>
          )}
        </div>
      ))}
      {edit && (
        <ProfileDialog
          supplierId={props.supplier.id}
          adapter={props.supplier.adapter}
          value={edit === 'new' ? undefined : edit}
          onClose={() => setEdit(null)}
        />
      )}
      {tariffs && (
        <TariffsDialog profile={tariffs} onClose={() => setTariffs(null)} />
      )}
      <ConfirmDialog
        open={Boolean(remove)}
        onOpenChange={(open) => {
          if (!open) setRemove(null)
        }}
        title={t('Delete price profile?')}
        desc={t(
          'Only unused profiles can be deleted. Disable a profile to retain its prices and history.'
        )}
        destructive
        isLoading={deletion.isPending}
        confirmText={t('Delete')}
        handleConfirm={() => {
          if (remove) deletion.mutate(remove.id)
        }}
      />
    </div>
  )
}
