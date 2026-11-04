import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/lib/api'

const referenceSchema = z.object({
  model_name: z.string().trim().min(1),
  input_usd: z.number().finite().nonnegative(),
  output_usd: z.number().finite().nonnegative(),
  source: z.url().startsWith('https://'),
  updated_at: z.number().int().positive(),
  price_unit: z.literal('million_tokens'),
})
type OfficialReference = z.infer<typeof referenceSchema>
type ReferenceResponse = {
  success: boolean
  message?: string
  data: OfficialReference[]
}
const queryKey = ['official-prices'] as const

function references(response: ReferenceResponse) {
  if (!response.success) throw new Error(response.message || 'Request failed')
  return response.data
}

export function OfficialPricesEditor() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<string | null>(null)
  const [invalid, setInvalid] = useState(false)
  const query = useQuery({
    queryKey,
    queryFn: async () =>
      references(
        (await api.get<ReferenceResponse>('/api/official-prices')).data
      ),
  })
  const update = (prices: OfficialReference[]) => {
    queryClient.setQueryData(queryKey, prices)
    setDraft(null)
    setInvalid(false)
    void queryClient.invalidateQueries({ queryKey: ['pricing'] })
  }
  const save = useMutation({
    mutationFn: async (prices: OfficialReference[]) =>
      references(
        (await api.put<ReferenceResponse>('/api/official-prices', { prices }))
          .data
      ),
    onSuccess: (prices) => {
      update(prices)
      toast.success(t('Reference prices saved'))
    },
  })
  const remove = useMutation({
    mutationFn: async (modelName: string) =>
      references(
        (
          await api.delete<ReferenceResponse>('/api/official-prices', {
            data: { model_name: modelName },
          })
        ).data
      ),
    onSuccess: (prices) => {
      update(prices)
      toast.success(t('Reference price removed'))
    },
  })
  const pending = save.isPending || remove.isPending
  const submit = () => {
    try {
      const parsed = z
        .array(referenceSchema)
        .min(1)
        .max(1000)
        .safeParse(JSON.parse(draft ?? '[]'))
      if (!parsed.success) {
        setInvalid(true)
        return
      }
      setInvalid(false)
      save.mutate(parsed.data)
    } catch {
      setInvalid(true)
    }
  }
  const addTemplate = () => {
    try {
      const value: unknown = JSON.parse(
        draft ?? JSON.stringify(query.data ?? [])
      )
      if (!Array.isArray(value)) {
        setInvalid(true)
        return
      }
      setDraft(
        JSON.stringify(
          [
            ...value,
            {
              model_name: '',
              input_usd: 0,
              output_usd: 0,
              source: '',
              updated_at: Math.floor(Date.now() / 1000),
              price_unit: 'million_tokens',
            },
          ],
          null,
          2
        )
      )
      setInvalid(false)
    } catch {
      setInvalid(true)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Official developer prices')}</CardTitle>
        <CardDescription>
          {t(
            "Reference API rates in USD per million tokens. Include the developer's public source URL and quote date in Unix seconds. References do not affect billing."
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-4'>
        <div className='space-y-2'>
          <Label htmlFor='official-prices-json'>
            {t('Reference prices JSON')}
          </Label>
          <Textarea
            id='official-prices-json'
            className='min-h-72 font-mono text-xs'
            value={draft ?? JSON.stringify(query.data ?? [], null, 2)}
            onChange={(event) => setDraft(event.target.value)}
            disabled={!query.data || pending}
            spellCheck={false}
            aria-invalid={invalid}
            aria-describedby={invalid ? 'official-prices-error' : undefined}
          />
          {invalid && (
            <p
              id='official-prices-error'
              role='alert'
              className='text-destructive text-sm'
            >
              {t('Enter a complete reference price JSON array')}
            </p>
          )}
          {query.isError && (
            <p role='alert' className='text-destructive text-sm'>
              {t('Failed to load prices')}
            </p>
          )}
          {query.isPending && (
            <p role='status' className='text-muted-foreground text-sm'>
              {t('Loading...')}
            </p>
          )}
        </div>
        <div className='flex flex-wrap gap-2'>
          <Button
            variant='outline'
            onClick={addTemplate}
            disabled={!query.data || pending}
          >
            {t('Add reference template')}
          </Button>
          <Button onClick={submit} disabled={draft === null || pending}>
            {t('Save')}
          </Button>
          <Button
            variant='ghost'
            disabled={draft === null || pending}
            onClick={() => {
              setDraft(null)
              setInvalid(false)
            }}
          >
            {t('Reset')}
          </Button>
        </div>
        {query.data?.map((price) => (
          <div
            key={price.model_name}
            className='flex min-w-0 flex-wrap items-center justify-between gap-2 border-t pt-3'
          >
            <div className='min-w-0'>
              <p className='font-medium break-all'>{price.model_name}</p>
              <p className='text-muted-foreground text-xs'>
                ${price.input_usd} / ${price.output_usd} ·{' '}
                {new Date(price.updated_at * 1000).toLocaleDateString()}
              </p>
              <a
                href={price.source}
                target='_blank'
                rel='noreferrer'
                className='text-muted-foreground block text-xs break-all underline'
              >
                {price.source}
              </a>
            </div>
            <Button
              variant='outline'
              size='sm'
              aria-label={`${t('Delete')}: ${price.model_name}`}
              disabled={pending || draft !== null}
              onClick={() => remove.mutate(price.model_name)}
            >
              {t('Delete')}
            </Button>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}
