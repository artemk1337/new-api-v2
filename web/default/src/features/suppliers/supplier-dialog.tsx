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

import { saveSupplier, supplierQueryKeys } from './api'
import type { Supplier } from './types'

const schema = z.object({
  code: z
    .string()
    .trim()
    .min(1)
    .max(64)
    .regex(/^[a-z0-9][a-z0-9-]*$/),
  name: z.string().trim().min(1).max(128),
  adapter: z.string().min(1),
  enabled: z.boolean(),
})

interface SupplierDialogProps {
  value?: Supplier
  onClose: () => void
}

export function SupplierDialog(props: SupplierDialogProps) {
  const { t } = useTranslation()
  const id = useId()
  const queryClient = useQueryClient()
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: props.value ?? {
      code: '',
      name: '',
      adapter: 'new-api',
      enabled: true,
    },
  })
  const save = useMutation({
    mutationFn: (value: z.infer<typeof schema>) =>
      saveSupplier(value, props.value?.id),
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
      title={t(props.value ? 'Edit supplier' : 'Add supplier')}
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
        <div className='grid gap-2'>
          <Label htmlFor={`${id}-code`}>{t('Code')}</Label>
          <Input
            id={`${id}-code`}
            placeholder='openlux'
            {...form.register('code')}
            aria-invalid={Boolean(form.formState.errors.code)}
          />
        </div>
        <div className='grid gap-2'>
          <Label htmlFor={`${id}-name`}>{t('Name')}</Label>
          <Input
            id={`${id}-name`}
            {...form.register('name')}
            aria-invalid={Boolean(form.formState.errors.name)}
          />
        </div>
        <div className='grid gap-2'>
          <Label htmlFor={`${id}-adapter`}>{t('Pricing adapter')}</Label>
          <NativeSelect
            id={`${id}-adapter`}
            className='w-full'
            {...form.register('adapter')}
          >
            {['new-api', 'openlux', 'bblabu', 'relay-fast', 'manual'].map(
              (adapter) => (
                <option key={adapter} value={adapter}>
                  {adapter}
                </option>
              )
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
        {!form.formState.isValid && form.formState.isSubmitted && (
          <p role='alert' className='text-destructive text-sm'>
            {t('Check the required fields')}
          </p>
        )}
      </form>
    </Dialog>
  )
}
