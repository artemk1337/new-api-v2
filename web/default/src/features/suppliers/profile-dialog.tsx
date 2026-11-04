import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useId } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'

import { saveSupplierProfile, supplierQueryKeys } from './api'
import type { SupplierPricingProfile } from './types'

const schema = z.object({
  code: z
    .string()
    .trim()
    .min(1)
    .max(64)
    .regex(/^[a-z0-9][a-z0-9-]*$/),
  name: z.string().trim().min(1).max(128),
  enabled: z.boolean(),
  base_url: z.string().trim(),
  endpoint: z.string().trim(),
  upstream_group: z.string().trim(),
  usd_per_credit: z.number().positive().finite(),
  interval_seconds: z.number().int().min(0),
})

interface ProfileDialogProps {
  supplierId: number
  adapter: string
  value?: SupplierPricingProfile
  onClose: () => void
}

export function ProfileDialog(props: ProfileDialogProps) {
  const { t } = useTranslation()
  const id = useId()
  const queryClient = useQueryClient()
  let defaultEndpoint = '/api/pricing'
  if (props.adapter === 'manual') defaultEndpoint = ''
  if (props.adapter === 'relay-fast') defaultEndpoint = '/pricing.json'
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: props.value ?? {
      code: '',
      name: '',
      enabled: true,
      base_url: '',
      endpoint: defaultEndpoint,
      upstream_group: '',
      interval_seconds: 0,
    },
  })
  const save = useMutation({
    mutationFn: (value: z.infer<typeof schema>) =>
      saveSupplierProfile(
        {
          ...value,
          supplier_id: props.supplierId,
          config_revision: props.value?.config_revision ?? 0,
        },
        props.value?.id
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: supplierQueryKeys.all })
      toast.success(t('Saved successfully'))
      props.onClose()
    },
  })

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={t(props.value ? 'Edit price profile' : 'Add price profile')}
      contentHeight='auto'
      footer={
        <>
          <Button variant='outline' onClick={props.onClose}>
            {t('Cancel')}
          </Button>
          <Button type='submit' form={id} disabled={save.isPending}>
            {t('Save')}
          </Button>
        </>
      }
    >
      <form
        id={id}
        className='space-y-4'
        onSubmit={form.handleSubmit((value) => save.mutate(value))}
      >
        <div className='grid gap-4 sm:grid-cols-2'>
          <div className='grid gap-2'>
            <Label htmlFor={`${id}-code`}>{t('Code')}</Label>
            <Input id={`${id}-code`} {...form.register('code')} />
          </div>
          <div className='grid gap-2'>
            <Label htmlFor={`${id}-name`}>{t('Name')}</Label>
            <Input id={`${id}-name`} {...form.register('name')} />
          </div>
        </div>
        <div className='grid gap-2'>
          <Label htmlFor={`${id}-url`}>{t('Base URL')}</Label>
          <Input
            id={`${id}-url`}
            placeholder='https://example.com'
            {...form.register('base_url')}
          />
        </div>
        <div className='grid gap-2'>
          <Label htmlFor={`${id}-endpoint`}>{t('Endpoint')}</Label>
          <Input id={`${id}-endpoint`} {...form.register('endpoint')} />
        </div>
        <div className='grid gap-2'>
          <Label htmlFor={`${id}-group`}>{t('Upstream group')}</Label>
          <Input id={`${id}-group`} {...form.register('upstream_group')} />
        </div>
        <div className='grid gap-2'>
          <Label htmlFor={`${id}-usd`}>
            {t('Real USD per provider credit')}
          </Label>
          <Input
            id={`${id}-usd`}
            type='number'
            min={0}
            step='any'
            placeholder='1'
            {...form.register('usd_per_credit', { valueAsNumber: true })}
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'Use the verified funding rate. Retail group multipliers apply after conversion to USD.'
            )}
          </p>
        </div>
        <div className='grid gap-2'>
          <Label htmlFor={`${id}-interval`}>{t('Sync interval')}</Label>
          <NativeSelect
            id={`${id}-interval`}
            className='w-full'
            {...form.register('interval_seconds', { valueAsNumber: true })}
          >
            <option value={0}>{t('Manual only')}</option>
            <option value={60}>{t('Every minute')}</option>
            <option value={600}>{t('Every 10 minutes')}</option>
            <option value={1800}>{t('Every 30 minutes')}</option>
            <option value={3600}>{t('Every hour')}</option>
            {props.value &&
              ![0, 60, 600, 1800, 3600].includes(
                props.value.interval_seconds
              ) && (
                <option value={props.value.interval_seconds}>
                  {props.value.interval_seconds} {t('seconds')}
                </option>
              )}
          </NativeSelect>
        </div>
        <div className='flex items-center justify-between gap-3'>
          <Label htmlFor={`${id}-enabled`}>{t('Enabled')}</Label>
          <Switch
            id={`${id}-enabled`}
            checked={form.watch('enabled')}
            onCheckedChange={(enabled) => form.setValue('enabled', enabled)}
          />
        </div>
        {form.formState.isSubmitted &&
          Object.keys(form.formState.errors).length > 0 && (
            <p role='alert' className='text-destructive text-sm'>
              {t('Check the required fields')}
            </p>
          )}
      </form>
    </Dialog>
  )
}
