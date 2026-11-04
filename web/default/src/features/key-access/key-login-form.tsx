import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { getKeySession, loginWithKey } from './api'

interface KeyLoginFormProps {
  className?: string
  disabled?: boolean
  onPendingChange?: (pending: boolean) => void
}

export function KeyLoginForm(props: KeyLoginFormProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [key, setKey] = useState('')
  const [pending, setPending] = useState(false)

  async function signIn(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending || props.disabled) return
    setPending(true)
    props.onPendingChange?.(true)
    try {
      await loginWithKey(key.trim())
      setKey('')
      useAuthStore.getState().auth.reset()
      window.localStorage.removeItem('uid')
      queryClient.clear()
      queryClient.setQueryData(['key-session'], await getKeySession())
      navigate({ to: '/key-access' })
    } catch {
      toast.error(t('Login failed'))
    } finally {
      setPending(false)
      props.onPendingChange?.(false)
    }
  }

  return (
    <form onSubmit={signIn} className={cn('grid gap-4', props.className)}>
      <div className='grid gap-2'>
        <Label htmlFor='api-key'>{t('API key')}</Label>
        <Input
          id='api-key'
          type='password'
          autoComplete='off'
          placeholder={t('API key')}
          required
          disabled={pending || props.disabled}
          value={key}
          onChange={(event) => setKey(event.target.value)}
        />
      </div>
      <p className='text-muted-foreground text-xs leading-5'>
        {t('View usage and remaining quota for this key.')}
      </p>
      <Button
        type='submit'
        className='mt-2 w-full'
        disabled={pending || props.disabled || !key.trim()}
      >
        {t('Sign in')}
      </Button>
    </form>
  )
}
