/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { AlertTriangle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import type { AuthUser } from '@/stores/auth-store'

type Props = { user: AuthUser | null }

export function AccountVerificationBanner({ user }: Props) {
  const { t } = useTranslation()
  if (!user?.verification_required && !user?.verification_frozen) return null

  const deadline = user.verification_deadline
    ? new Date(user.verification_deadline * 1000).toLocaleDateString()
    : null

  return (
    <Alert variant={user.verification_frozen ? 'destructive' : 'default'}>
      <AlertTriangle />
      <AlertTitle>
        {user.verification_frozen
          ? t('Account verification is overdue')
          : t('Account verification required')}
      </AlertTitle>
      <AlertDescription>
        {deadline
          ? t(
              'Please verify your account by {{date}}. Otherwise, it will be frozen and deleted after 30 days.',
              { date: deadline }
            )
          : t(
              'Please verify your account. Unverified accounts are deleted after 30 days.'
            )}
      </AlertDescription>
    </Alert>
  )
}
