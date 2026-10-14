/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { z } from 'zod'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import type { ReceivingWallet } from './receiving-wallets-editor'

type Props = {
  wallet: ReceivingWallet
  editing: boolean
  schema: z.ZodType
  onClose: () => void
  onSave: (wallet: ReceivingWallet) => void
}

export function ReceivingWalletDialog(props: Props) {
  const { t } = useTranslation()
  const id = useId()
  const [wallet, setWallet] = useState(props.wallet)
  const [submitted, setSubmitted] = useState(false)
  const result = props.schema.safeParse([wallet])
  const issues = !result.success && submitted ? result.error.issues : []
  const change = (patch: Partial<ReceivingWallet>) =>
    setWallet({ ...wallet, ...patch })
  const save = () => {
    setSubmitted(true)
    if (result.success) props.onSave(wallet)
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={props.editing ? t('Edit wallet') : t('Add wallet')}
      description={t('USDT receiving wallet pool')}
      contentClassName='sm:max-w-[500px]'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button type='button' variant='outline' onClick={props.onClose}>
            {t('Cancel')}
          </Button>
          <Button type='button' onClick={save}>
            {props.editing ? t('Update') : t('Add')}
          </Button>
        </>
      }
    >
      <div className='grid gap-3 sm:grid-cols-2'>
        <div className='space-y-2'>
          <Label htmlFor={`${id}-key`}>{t('Wallet identifier')}</Label>
          <Input
            id={`${id}-key`}
            value={wallet.key ?? ''}
            placeholder='tron-1'
            onChange={(event) => change({ key: event.target.value })}
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor={`${id}-network`}>{t('Network')}</Label>
          <Select
            value={wallet.network}
            onValueChange={(network) => {
              if (network) change({ network })
            }}
          >
            <SelectTrigger id={`${id}-network`} className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent
              onKeyDown={(event) => {
                if (event.key === 'Escape') event.stopPropagation()
              }}
            >
              {['TRON', 'TON', 'SOLANA'].map((network) => (
                <SelectItem key={network} value={network}>
                  {network}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {(['address', 'owner', 'destination'] as const)
          .filter(
            (field) =>
              field === 'address' || wallet.network !== 'TRON' || wallet[field]
          )
          .map((field) => {
            const error = issues.find(
              (issue) => issue.path[0] === 0 && issue.path[1] === field
            )
            let label = t('Receiving address')
            if (field === 'owner') label = t('Wallet owner address')
            if (field === 'destination') {
              label = t('Token account address')
            }
            return (
              <div key={field} className='space-y-2 sm:col-span-2'>
                <Label htmlFor={`${id}-${field}`}>{label}</Label>
                <Input
                  id={`${id}-${field}`}
                  value={wallet[field] ?? ''}
                  aria-invalid={!!error}
                  aria-describedby={error ? `${id}-${field}-error` : undefined}
                  onChange={(event) => change({ [field]: event.target.value })}
                />
                {error && (
                  <p
                    id={`${id}-${field}-error`}
                    className='text-destructive text-sm'
                  >
                    {t(error.message)}
                  </p>
                )}
              </div>
            )
          })}
      </div>
      <div className='flex items-center gap-2'>
        <Switch
          id={`${id}-enabled`}
          checked={wallet.enabled !== false}
          onCheckedChange={(enabled) => change({ enabled })}
        />
        <Label htmlFor={`${id}-enabled`}>{t('Enabled')}</Label>
      </div>
    </Dialog>
  )
}
