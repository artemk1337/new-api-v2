import { createFileRoute } from '@tanstack/react-router'

import { PublicLayout } from '@/components/layout'
import { KeyAccess } from '@/features/key-access'

export const Route = createFileRoute('/key-access')({
  component: () => (
    <PublicLayout showMainContainer={false}>
      <KeyAccess />
    </PublicLayout>
  ),
})
