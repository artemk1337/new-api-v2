import { createFileRoute } from '@tanstack/react-router'

import { KeyAccess } from '@/features/key-access'

export const Route = createFileRoute('/key-access')({ component: KeyAccess })
