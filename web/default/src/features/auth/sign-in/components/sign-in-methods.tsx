import { Tabs } from '@base-ui/react/tabs'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { KeyLoginForm } from '@/features/key-access'

import '@/styles/sign-in.css'

interface SignInMethodsProps {
  passwordLoginEnabled: boolean
  disabled: boolean
  onKeyPendingChange: (pending: boolean) => void
  children: ReactNode
}

export function SignInMethods(props: SignInMethodsProps) {
  const { t } = useTranslation()

  const keyLoginForm = (
    <KeyLoginForm
      disabled={props.disabled}
      onPendingChange={props.onKeyPendingChange}
    />
  )

  if (!props.passwordLoginEnabled) return keyLoginForm

  return (
    <Tabs.Root defaultValue='password' className='grid gap-6'>
      <Tabs.List className='sign-in-method-list' aria-label={t('Sign in')}>
        <Tabs.Tab
          value='password'
          disabled={props.disabled}
          className='sign-in-method-tab'
        >
          {t('Login and password')}
        </Tabs.Tab>
        <Tabs.Tab
          value='key'
          disabled={props.disabled}
          className='sign-in-method-tab'
        >
          {t('API key')}
        </Tabs.Tab>
        <Tabs.Indicator className='sign-in-method-indicator' />
      </Tabs.List>
      <div className='grid'>
        <Tabs.Panel
          value='password'
          keepMounted
          className='sign-in-method-panel'
          render={(panelProps, state) => (
            <div
              {...panelProps}
              hidden={undefined}
              aria-hidden={state.hidden}
            />
          )}
        >
          {props.children}
        </Tabs.Panel>
        <Tabs.Panel
          value='key'
          keepMounted
          className='sign-in-method-panel'
          render={(panelProps, state) => (
            <div
              {...panelProps}
              hidden={undefined}
              aria-hidden={state.hidden}
            />
          )}
        >
          {keyLoginForm}
        </Tabs.Panel>
      </div>
    </Tabs.Root>
  )
}
