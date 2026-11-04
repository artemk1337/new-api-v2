import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'

import { getSupplierMigrationPreview } from './api'

export function SupplierMigrationPreview(props: { onClose: () => void }) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['suppliers', 'migration-preview'],
    queryFn: getSupplierMigrationPreview,
  })
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={t('Supplier migration preview')}
      description={t(
        'Review existing tags, groups, and sources before binding them. This preview makes no changes.'
      )}
      contentHeight='auto'
      footer={
        <Button variant='outline' onClick={props.onClose}>
          {t('Close')}
        </Button>
      }
    >
      {query.isLoading && (
        <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
      )}
      {query.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Failed to load migration preview')}
        </p>
      )}
      {query.data && (
        <pre className='bg-muted max-h-96 overflow-auto rounded-lg p-4 text-xs break-words whitespace-pre-wrap'>
          {JSON.stringify(query.data, null, 2)}
        </pre>
      )}
    </Dialog>
  )
}
