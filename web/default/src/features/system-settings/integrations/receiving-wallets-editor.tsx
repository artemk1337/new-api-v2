import { Plus, Search } from 'lucide-react'
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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

import { ReceivingWalletDialog } from './receiving-wallet-dialog'

const editableWalletsSchema = z.array(
  z
    .object({
      key: z.string().optional(),
      network: z
        .string()
        .trim()
        .toUpperCase()
        .pipe(z.enum(['TRON', 'TON', 'SOLANA'])),
      address: z.string(),
      owner: z.string().optional(),
      destination: z.string().optional(),
      enabled: z.boolean().nullable().optional(),
    })
    .passthrough()
)
export type ReceivingWallet = z.infer<typeof editableWalletsSchema>[number]
type Props = {
  value: string
  onChange: (value: string) => void
  onBlur: () => void
  schema: z.ZodType
}

export function ReceivingWalletsEditor(props: Props) {
  const { t } = useTranslation()
  const [jsonMode, setJsonMode] = useState(false)
  const [search, setSearch] = useState('')
  const [edit, setEdit] = useState<{
    index: number
    wallet: ReceivingWallet
  } | null>(null)
  let wallets: ReceivingWallet[] | null = null
  try {
    const result = editableWalletsSchema.safeParse(
      JSON.parse(props.value.trim() || '[]')
    )
    if (result.success) wallets = result.data
  } catch {
    // Preserve malformed JSON for correction.
  }
  const showJson = jsonMode || wallets === null
  const update = (next: ReceivingWallet[]) =>
    props.onChange(next.length ? JSON.stringify(next, null, 2) : '')
  const rows = (wallets ?? [])
    .map((wallet, index) => ({ wallet, index }))
    .filter(({ wallet }) =>
      [
        wallet.key,
        wallet.network,
        wallet.address,
        wallet.owner,
        wallet.destination,
      ].some((value) => value?.toLowerCase().includes(search.toLowerCase()))
    )
  const actions = (row: (typeof rows)[number]) => (
    <StaticRowActions
      editLabel={`${t('Edit wallet')} ${row.wallet.key || row.index + 1}`}
      deleteLabel={t('Remove wallet')}
      menuLabel={t('Actions')}
      onEdit={() => setEdit({ index: row.index, wallet: row.wallet })}
      onDelete={() =>
        update((wallets ?? []).filter((_, index) => index !== row.index))
      }
    />
  )
  return (
    <div className='space-y-4' onBlur={props.onBlur}>
      <div className='flex justify-end'>
        <Button
          type='button'
          variant='outline'
          disabled={wallets === null}
          onClick={() => setJsonMode(!jsonMode)}
        >
          {showJson ? t('Visual Editor') : t('JSON Editor')}
        </Button>
      </div>
      {showJson ? (
        <Textarea
          aria-label={t('USDT receiving wallet pool')}
          className='font-mono'
          rows={8}
          value={props.value}
          onChange={(event) => props.onChange(event.target.value)}
        />
      ) : (
        <>
          <div className='flex flex-col gap-3 sm:flex-row sm:items-center'>
            <div className='relative flex-1'>
              <Search
                aria-hidden='true'
                className='text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4'
              />
              <Input
                aria-label={t('Search wallets...')}
                placeholder={t('Search wallets...')}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                className='pl-9'
              />
            </div>
            <Button
              type='button'
              variant='outline'
              onClick={() =>
                setEdit({
                  index: -1,
                  wallet: {
                    key: '',
                    network: 'TRON',
                    address: '',
                    enabled: true,
                  },
                })
              }
            >
              <Plus aria-hidden='true' className='h-4 w-4' />
              {t('Add wallet')}
            </Button>
          </div>
          {rows.length === 0 ? (
            <div className='text-muted-foreground rounded-lg border border-dashed p-8 text-center text-sm'>
              {search
                ? t('No wallets match your search')
                : t(
                    'No wallet pool configured. Network addresses below are used.'
                  )}
            </div>
          ) : (
            <div className='rounded-md border'>
              <StaticDataTable
                className='hidden rounded-none border-0 md:block'
                data={rows}
                getRowKey={(row) => row.index}
                columns={[
                  {
                    id: 'key',
                    header: t('Wallet identifier'),
                    cellClassName: 'font-medium',
                    cell: (row) =>
                      row.wallet.key || `${t('Wallet')} ${row.index + 1}`,
                  },
                  {
                    id: 'network',
                    header: t('Network'),
                    cell: (row) => row.wallet.network,
                  },
                  {
                    id: 'address',
                    header: t('Receiving address'),
                    cellClassName: 'max-w-64 font-mono text-xs',
                    cell: (row) => row.wallet.address,
                  },
                  {
                    id: 'enabled',
                    header: t('Status'),
                    cell: (row) =>
                      row.wallet.enabled === false
                        ? t('Disabled')
                        : t('Enabled'),
                  },
                  {
                    id: 'actions',
                    header: t('Actions'),
                    className: 'text-right',
                    cell: actions,
                  },
                ]}
              />
              <div className='divide-y md:hidden'>
                {rows.map((row) => (
                  <div key={String(row.index)} className='space-y-2 p-4'>
                    <div className='flex items-start justify-between gap-2'>
                      <div>
                        <div className='font-medium'>
                          {row.wallet.key || `${t('Wallet')} ${row.index + 1}`}
                        </div>
                        <div className='text-muted-foreground text-xs'>
                          {row.wallet.network} ·{' '}
                          {row.wallet.enabled === false
                            ? t('Disabled')
                            : t('Enabled')}
                        </div>
                      </div>
                      {actions(row)}
                    </div>
                    <div className='text-muted-foreground font-mono text-xs break-all'>
                      {row.wallet.address}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </>
      )}
      {wallets === null && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Enter a JSON array of receiving wallets')}
        </p>
      )}
      {edit && (
        <ReceivingWalletDialog
          wallet={edit.wallet}
          editing={edit.index >= 0}
          schema={props.schema}
          onClose={() => setEdit(null)}
          onSave={(wallet) => {
            const next = [...(wallets ?? [])]
            if (edit.index < 0) next.push(wallet)
            else next[edit.index] = wallet
            update(next)
            setEdit(null)
          }}
        />
      )}
    </div>
  )
}
